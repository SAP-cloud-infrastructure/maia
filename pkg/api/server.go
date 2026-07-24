// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/spf13/viper"

	"github.com/sapcc/go-bits/httpext"
	"github.com/sapcc/go-bits/logg"

	"github.com/SAP-cloud-infrastructure/maia/pkg/keystone"
	"github.com/SAP-cloud-infrastructure/maia/pkg/storage"
	"github.com/SAP-cloud-infrastructure/maia/pkg/ui"
	newui "github.com/SAP-cloud-infrastructure/maia/web/ui"
)

var storageInstance storage.Driver
var keystoneInstance keystone.Driver
var globalKeystoneInstance keystone.Driver

// sentinelValue is the configured global visibility sentinel, resolved once at
// startup. When non-empty, it is appended to project_id/domain_id scope
// constraints so that metrics carrying this value are visible to all tenants.
var sentinelValue string

// Server initializes and starts the API server, hooking it up to the API router
func Server(ctx context.Context) error {
	prometheusAPIURL := viper.GetString("maia.prometheus_url")
	if prometheusAPIURL == "" {
		panic(errors.New("prometheus endpoint not configured (maia.prometheus_url / MAIA_PROMETHEUS_URL)"))
	}

	// Initialize regular keystone driver
	keystoneDriver := keystone.NewKeystoneDriver()

	// Initialize global keystone if configured
	var globalKeystone keystone.Driver
	if viper.IsSet("keystone.global.auth_url") {
		logg.Info("Initializing global Keystone connection to %s", viper.GetString("keystone.global.auth_url"))
		globalKeystone = keystone.NewKeystoneDriverWithSection("global")
		globalKeystoneInstance = globalKeystone
	}

	// Validate and resolve sentinel value for global metric visibility (once at startup)
	sentinelValue = viper.GetString("maia.label_value_for_global_visibility")
	if sentinelValue != "" {
		if regexp.QuoteMeta(sentinelValue) != sentinelValue {
			panic(fmt.Errorf("maia.label_value_for_global_visibility contains regex metacharacters (value: %q); it must be a plain literal because it is injected into scope regexes verbatim", sentinelValue))
		}
		logg.Info("Global metric visibility sentinel configured: %q (appended to project_id/domain_id scope constraints)", sentinelValue)
	}

	// The main router dispatches all incoming requests
	mainRouter := setupRouter(keystoneDriver, globalKeystone, storage.NewPrometheusDriver(prometheusAPIURL, map[string]string{}))

	bindAddress := viper.GetString("maia.bind_address")
	logg.Info("listening on %s", bindAddress)

	// enable CORS
	c := cors.New(cors.Options{
		AllowedHeaders: []string{"X-Auth-Token", "X-Global-Region"},
	})
	handler := c.Handler(mainRouter)

	// start HTTP server and block; shuts down gracefully when ctx is cancelled
	return httpext.ListenAndServeContext(ctx, bindAddress, handler)
}

// setupRouter initializes the main http router
func setupRouter(keystoneDriver, globalKeystoneDriver keystone.Driver, storageDriver storage.Driver) http.Handler {
	storageInstance = storageDriver
	keystoneInstance = keystoneDriver
	globalKeystoneInstance = globalKeystoneDriver

	mainRouter := mux.NewRouter()

	// Add keystone resolution middleware early in the chain
	// This prevents race conditions by determining keystone instance once per request
	mainRouter.Use(keystoneResolutionMiddleware)

	mainRouter.Methods(http.MethodGet).Path("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if viper.GetBool("maia.new_ui_enabled") {
			http.Redirect(w, r, "/ui/query", http.StatusFound)
		} else {
			redirectToRootPage(w, r)
		}
	})

	// Readiness probe used by the React UI's ReadinessWrapper
	mainRouter.Methods(http.MethodGet).Path("/-/ready").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Maia is Ready."))
	})

	// the API is versioned, other paths are not
	apiRouter := mainRouter.PathPrefix("/api/").Subrouter()
	mainRouter.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		allVersions := struct {
			Versions []VersionData `json:"versions"`
		}{[]VersionData{versionData()}}
		ReturnJSON(w, http.StatusMultipleChoices, allVersions)
	})
	// hook up the v1 API (this code is structured so that a newer API version can
	// be added easily later)
	v1Handler := NewV1Handler(keystoneDriver, storageDriver)
	apiRouter.PathPrefix("/v1/").Handler(http.StripPrefix("/api/v1", v1Handler))

	// other endpoints
	// maia's federate endpoint
	mainRouter.Methods(http.MethodGet).Path("/federate").HandlerFunc(
		authorize(observeDuration(Federate, "federate"), false, "metric:show"))
	// expression browser
	mainRouter.Methods(http.MethodGet).PathPrefix("/static/").HandlerFunc(serveStaticContent)
	mainRouter.Methods(http.MethodGet).PathPrefix("/favicon.ico").HandlerFunc(serveStaticContent)
	mainRouter.Methods(http.MethodGet).Path("/graph").HandlerFunc(redirectToRootPage)
	// scrape endpoint for Prometheus
	mainRouter.Handle("/metrics", promhttp.Handler())

	// domain-prefixed paths. Order is relevant! This implies that there must be no domain federate, static or graph :-)
	mainRouter.Methods(http.MethodGet).Path("/{domain}/graph").HandlerFunc(authorize(observeDuration(observeResponseSize(graph, "graph"), "graph"), true, "metric:show"))
	mainRouter.Methods(http.MethodGet).Path("/{domain}").HandlerFunc(redirectToDomainRootPage)

	// P2-2/P2-3: new React UI routes, gated by maia.new_ui_enabled
	if viper.GetBool("maia.new_ui_enabled") {
		mainRouter.Methods(http.MethodGet).Path("/ui").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/ui/query", http.StatusFound)
		})
		// Strip /ui/ prefix; MantineUIAssets is rooted at mantine-ui/
		// so /ui/assets/foo.js → assets/foo.js inside MantineUIAssets
		uiFileServer := http.StripPrefix("/ui/", http.FileServer(newui.MantineUIAssets))
		mainRouter.Methods(http.MethodGet).PathPrefix("/ui/assets/").Handler(uiFileServer)
		mainRouter.Methods(http.MethodGet).Path("/ui/favicon.svg").Handler(uiFileServer)
		mainRouter.Methods(http.MethodGet).Path("/ui/manifest.json").Handler(uiFileServer)
		// SPA catch-all: all other /ui/* paths get index.html
		mainRouter.Methods(http.MethodGet).PathPrefix("/ui/").HandlerFunc(serveReactApp)
	}

	// provide the inflight metrics for all paths
	return gaugeInflight(mainRouter)
}

var validDomain = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// trueValue is required by golangci-lint when string literals appear 5+ times
// Alternative would be multiple //nolint:goconst annotations which is messier
const trueValue = "true"

// redirectToDomainRootPage will redirect users to the UI start page for their domain
func redirectToDomainRootPage(w http.ResponseWriter, r *http.Request) {
	domain, ok := mux.Vars(r)["domain"]
	if !ok || !validDomain.MatchString(domain) {
		logg.Debug("Invalid domain: %s", domain)
		redirectToRootPage(w, r)
		return
	}

	// Preserve existing query parameters
	q := r.URL.Query()

	// Check if global flag is set in header but not in query params
	if r.Header.Get("X-Global-Region") == trueValue && q.Get("global") == "" {
		q.Set("global", trueValue)
	}

	// Encode domain to prevent any potential attacks
	domain = url.PathEscape(domain)

	// Construct redirect URL with preserved query parameters
	target := "//" + r.Host + "/" + domain + "/graph"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}

	logg.Debug("Redirecting %s to %s", r.URL.Path, target)
	http.Redirect(w, r, target, http.StatusFound) //nolint:gosec // G710: r.Host is set by the reverse proxy, not user-controlled
}

// redirectToRootPage will redirect users to the global start page
func redirectToRootPage(w http.ResponseWriter, r *http.Request) {
	domain := viper.GetString("keystone.default_user_domain_name")
	username, _, ok := r.BasicAuth()
	if ok && strings.Contains(strings.Split(username, "|")[0], "@") {
		domain = strings.Split(username, "@")[1]
		logg.Debug("Username contains domain info. Redirecting to domain %s", domain)
	}

	// Preserve existing query parameters
	q := r.URL.Query()

	// Check if global flag is set in header but not in query params
	if r.Header.Get("X-Global-Region") == trueValue && q.Get("global") == "" {
		q.Set("global", trueValue)
	}

	// Construct redirect URL with preserved query parameters
	target := "//" + r.Host + "/" + domain + "/graph"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}

	logg.Debug("Redirecting to %s", target)
	http.Redirect(w, r, target, http.StatusFound) //nolint:gosec // G710: r.Host is set by the reverse proxy, not user-controlled
}

// serveStaticContent serves all the static assets of the web UI (pages, js, images)
func serveStaticContent(w http.ResponseWriter, req *http.Request) {
	fp := req.URL.Path
	if fp == "/favicon.ico" {
		// support favicon web standard
		fp = filepath.Join("static", "img", fp)
	}
	fp = filepath.Join("web", fp)

	info, err := ui.AssetInfo(fp)
	if err != nil {
		logg.Info("WARNING: Could not get file info: %v", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	file, err := ui.Asset(fp)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			logg.Info("WARNING: Could not get file info: %v", err)
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}

	http.ServeContent(w, req, info.Name(), info.ModTime(), bytes.NewReader(file))
}

// Federate handles GET /federate.
func Federate(w http.ResponseWriter, req *http.Request) {
	// Get keystone from context (secure, race-condition-free approach)
	ks := getKeystoneFromContext(req.Context())
	if ks == nil {
		// Context-based keystone resolution is mandatory for security
		logg.Error("Missing keystone context in Federate - request may have bypassed keystoneResolutionMiddleware")
		ReturnPromError(w, errors.New("keystone context not available"), http.StatusInternalServerError)
		return
	}

	selectors, err := buildSelectors(req, ks)
	if err != nil {
		logg.Info("Invalid request params %s", req.URL)
		ReturnPromError(w, err, http.StatusBadRequest)
		return
	}

	response, err := storageInstance.Federate(*selectors, req.Header.Get("Accept"))
	if err != nil {
		logg.Error("Could not get metrics for %s", selectors)
		ReturnPromError(w, err, http.StatusServiceUnavailable)
		return
	}

	ReturnResponse(w, response)
}

// serveReactApp serves the Maia React UI SPA for all /ui/* paths.
// It reads index.html from the embedded assets and replaces Prometheus
// placeholders with Maia-appropriate values before writing the response.
func serveReactApp(w http.ResponseWriter, req *http.Request) {
	f, err := newui.MantineUIAssets.Open("index.html")
	if err != nil {
		http.Error(w, "UI not available", http.StatusNotFound)
		return
	}
	defer f.Close()

	html, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "failed to read UI", http.StatusInternalServerError)
		return
	}

	html = bytes.ReplaceAll(html, []byte("TITLE_PLACEHOLDER"), []byte("Maia"))
	html = bytes.ReplaceAll(html, []byte("AGENT_MODE_PLACEHOLDER"), []byte("false"))
	html = bytes.ReplaceAll(html, []byte("READY_PLACEHOLDER"), []byte("true"))
	html = bytes.ReplaceAll(html, []byte("CONSOLES_LINK_PLACEHOLDER"), []byte(""))
	html = bytes.ReplaceAll(html, []byte("LOOKBACKDELTA_PLACEHOLDER"), []byte(""))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(html)
}

// graph returns the Prometheus UI page
func graph(w http.ResponseWriter, req *http.Request) {
	// Get keystone from context (secure, race-condition-free approach)
	ks := getKeystoneFromContext(req.Context())
	if ks == nil {
		// Context-based keystone resolution is mandatory for security
		logg.Error("Missing keystone context in graph - request may have bypassed keystoneResolutionMiddleware")
		http.Error(w, "Internal server error: keystone context not available", http.StatusInternalServerError)
		return
	}
	// P2-4: pass new UI flag so the template can show the "Try it →" banner
	data := map[string]any{
		"newUIEnabled": viper.GetBool("maia.new_ui_enabled"),
	}
	ui.ExecuteTemplate(w, req, "graph.html", ks, data)
}
