// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"bytes"
	"io"
	"path/filepath"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/spf13/viper"

	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/maia/pkg/keystone"
	"github.com/sapcc/maia/pkg/storage"
	"github.com/sapcc/maia/pkg/ui"
)

var storageInstance storage.Driver
var keystoneInstance keystone.Driver
var globalKeystoneInstance keystone.Driver

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

	// The main router dispatches all incoming requests
	mainRouter := setupRouter(keystoneDriver, globalKeystone, storage.NewPrometheusDriver(prometheusAPIURL, map[string]string{}))

	bindAddress := viper.GetString("maia.bind_address")
	logg.Info("listening on %s", bindAddress)

	// enable CORS
	c := cors.New(cors.Options{
		AllowedHeaders: []string{"X-Auth-Token", "X-Global-Region"},
	})
	handler := c.Handler(mainRouter)

	// start HTTP server and block
	return http.ListenAndServe(bindAddress, handler) //nolint:gosec // TODO: use httpext.ListenAndServeContext() from go-bits
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

	mainRouter.Methods(http.MethodGet).Path("/").HandlerFunc(redirectToRootPage)

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
	http.Redirect(w, r, target, http.StatusFound)
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
	http.Redirect(w, r, target, http.StatusFound)
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
	ui.ExecuteTemplate(w, req, "graph.html", ks, nil)
}
