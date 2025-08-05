// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	policy "github.com/databus23/goslo.policy"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"

	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/maia/pkg/keystone"
	"github.com/sapcc/maia/pkg/storage"
	"github.com/sapcc/maia/pkg/util"
)

// utility functionality

// contextKey is a custom type to prevent collisions with other packages
// that might use string keys in context.Context. This is a Go best practice
// that ensures our keystone selection context keys remain isolated.
type contextKey string

const (
	keystoneTypeKey     contextKey = "maia.keystone.type"
	keystoneInstanceKey contextKey = "maia.keystone.instance"
)

// VersionData is used by version advertisement handlers.
type VersionData struct {
	Status string            `json:"status"`
	ID     string            `json:"id"`
	Links  []versionLinkData `json:"links"`
}

// versionLinkData is used by version advertisement handlers, as part of the
// VersionData struct.
type versionLinkData struct {
	URL      string `json:"href"`
	Relation string `json:"rel"`
	Type     string `json:"type,omitempty"`
}

const authTokenCookieName = "X-Auth-Token" //nolint:gosec //not a credential
const userDomainCookieName = "X-User-Domain-Name"
const authTokenHeader = "X-Auth-Token" //nolint:gosec //not a credential
const userDomainHeader = "X-User-Domain-Name"
const authTokenExpiryHeader = "X-Auth-Token-Expiry" //nolint:gosec //not a credential

var policyEnforcer *policy.Enforcer
var authErrorsCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "maia_logon_errors_count", Help: "Number of logon errors occurred in Maia"})
var authFailuresCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "maia_logon_failures_count", Help: "Number of logon attempts failed due to wrong credentials"})
var promErrorsCounter = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "maia_tsdb_errors_count", Help: "Number of technical errors occurred when accessing Maia's underlying TSDB (i.e. Prometheus)"})

func init() {
	prometheus.MustRegister(authErrorsCounter, authFailuresCounter, promErrorsCounter)
}

// provides version data
func versionData() VersionData {
	return VersionData{
		Status: "CURRENT",
		ID:     "v1",
		Links: []versionLinkData{
			{
				Relation: "self",
				URL:      keystoneInstance.ServiceURL(),
			},
			{
				Relation: "describedby",
				URL:      "https://github.com/sapcc/maia/tree/master/README.md",
				Type:     "text/html",
			},
		},
	}
}

// ReturnResponse basically forwards a received Response.
func ReturnResponse(w http.ResponseWriter, response *http.Response) {
	defer response.Body.Close()

	// copy headers
	for k, v := range response.Header {
		w.Header().Set(k, strings.Join(v, ";"))
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(response.Body) //nolint:errcheck // TODO go-bits replacement?

	body := buf.String()
	w.WriteHeader(response.StatusCode)

	io.WriteString(w, body) //nolint:errcheck // TODO go-bits? otherwise I can make return response return an err
}

// ReturnJSON is a convenience function for HTTP handlers returning JSON data.
// The `code` argument specifies the HTTP Response code, usually 200.
func ReturnJSON(w http.ResponseWriter, code int, data any) {
	payload, err := json.Marshal(&data)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		// restore "&" in links that are broken by the json.Marshaller
		payload = bytes.ReplaceAll(payload, []byte("\\u0026"), []byte("&"))
		_, err = w.Write(payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// ReturnPromError produces a Prometheus error Response with HTTP Status code
func ReturnPromError(w http.ResponseWriter, err error, code int) {
	if code >= 500 {
		promErrorsCounter.Add(1)
	}

	var errorType storage.ErrorType
	switch code {
	case http.StatusBadRequest:
		errorType = storage.ErrorBadData
	case http.StatusUnprocessableEntity:
		errorType = storage.ErrorExec
	case http.StatusServiceUnavailable:
		errorType = storage.ErrorTimeout
	default:
		errorType = storage.ErrorInternal
	}

	jsonErr := storage.Response{Status: storage.StatusError, ErrorType: errorType, Error: err.Error()}
	ReturnJSON(w, code, jsonErr)
}

func scopeToLabelConstraint(req *http.Request, keystoneDriver keystone.Driver) (string, []string) { //nolint:gocritic
	ctx := req.Context()
	if projectID := req.Header.Get("X-Project-Id"); projectID != "" {
		children, err := keystoneDriver.ChildProjects(ctx, projectID)
		if err != nil {
			panic(err)
		}
		return "project_id", append([]string{projectID}, children...)
	} else if domainID := req.Header.Get("X-Domain-Id"); domainID != "" {
		return "domain_id", []string{domainID}
	}

	panic(errors.New("missing OpenStack scope attributes in request header"))
}

// buildSelectors takes the selectors contained in the "match[]" URL query parameter(s)
// and extends them with a label-constrained for the project/domain scope
func buildSelectors(req *http.Request, keystoneDriver keystone.Driver) (*[]string, error) {
	labelKey, labelValues := scopeToLabelConstraint(req, keystoneDriver)

	queryParams := req.URL.Query()
	selectors := queryParams["match[]"]
	if selectors == nil {
		// behave like Prometheus, but do not proxy through
		return nil, errors.New("no match[] parameter provided")
	}
	// enrich all match statements
	for i, sel := range selectors {
		newSel, err := util.AddLabelConstraintToSelector(sel, labelKey, labelValues)
		if err != nil {
			return nil, err
		}
		selectors[i] = newSel
	}

	return &selectors, nil
}

func policyEngine() *policy.Enforcer {
	if policyEnforcer != nil {
		return policyEnforcer
	}

	// set up policy engine lazily
	filebytes, err := os.ReadFile(viper.GetString("keystone.policy_file"))
	if err != nil {
		panic(fmt.Errorf("policy file %s not found: %w", viper.GetString("keystone.policy_file"), err))
	}
	var rules map[string]string
	err = json.Unmarshal(filebytes, &rules)
	if err != nil {
		panic(err)
	}
	policyEnforcer, err = policy.NewEnforcer(rules)
	if err != nil {
		panic(err)
	}

	return policyEnforcer
}

func isPlainBasicAuth(req *http.Request) bool {
	if username, _, ok := req.BasicAuth(); ok {
		return !strings.ContainsAny(username, "@|")
	}
	return false
}

func authorizeRules(keystoneDriver keystone.Driver, w http.ResponseWriter, req *http.Request, guessScope bool, rules []string) bool {
	logg.Debug("authenticate")
	matchedRules := []string{}

	domain, domainSet := mux.Vars(req)["domain"]

	// 1. check token cookies, then user-domain specified via path prefix or cookie
	cookie, cookieErr := req.Cookie(authTokenCookieName)
	cookieSet := false
	if cookieErr == nil && cookie.Value != "" && req.Header.Get(authTokenHeader) == "" {
		logg.Debug("found token cookie: %s...", cookie.String()[:1+len(cookie.String())/4])
		req.Header.Set(authTokenHeader, cookie.Value)
		cookieSet = true
	} else if isPlainBasicAuth(req) {
		// if username is not qualified and scoped we might need to cookie to interpret the username right
		if !domainSet {
			cookie, err := req.Cookie(userDomainCookieName)
			if err == nil && cookie.Value != "" && req.Header.Get(userDomainHeader) == "" {
				domain = cookie.Value
				domainSet = true
			}
			logg.Debug("setting user domain via cookie: %s", domain)
		} else {
			logg.Debug("setting user domain via URL: %s", domain)
		}
		req.Header.Set(userDomainHeader, domain)
	}

	// 2. authenticate
	ctx := req.Context()
	policyContext, err := keystoneDriver.AuthenticateRequest(ctx, req, guessScope)
	if err != nil {
		code := err.StatusCode()
		httpCode := http.StatusUnauthorized

		switch code {
		case keystone.StatusWrongCredentials:
			authFailuresCounter.Add(1)
			// expire the cookie and ask for new credentials if they are wrong
			username, _, ok := req.BasicAuth()
			if !ok {
				username = req.UserAgent()
			}
			logg.Info("Request with wrong credentials from %s: %s", username, err.Error())
			requestReauthentication(w)
		case keystone.StatusMissingCredentials:
			requestReauthentication(w)
		case keystone.StatusNoPermission:
			httpCode = http.StatusForbidden
		default:
			// warn of possible technical issues
			logg.Info("WARNING: Authentication error: %s", err.Error())
			authErrorsCounter.Add(1)
			httpCode = http.StatusInternalServerError
		}

		http.Error(w, err.Error(), httpCode)
		return false
	} else if domainSet && req.Header.Get("X-User-Domain-Name") != domain {
		// authentication was successful, but do the credentials match the given domain or do they perhaps belong to another user? we could not know in advance
		// either the basic authentication credentials or the cookie do not match the domain in the URL
		if cookieSet {
			// there is a cookie: clear it and ask for new credentials
			logg.Debug("User domain mismatch between %s (cookie with token) and %s (URL)", req.Header.Get("X-User-Domain-Name"), domain)
			requestReauthentication(w)
			http.Error(w, "User switch: please login again", http.StatusUnauthorized)
		} else {
			// redirect to the domain that fits the user credentials
			redirectToDomainRootPage(w, req)
		}
		return false
	}

	// 3. authorize
	pe := policyEngine()
	for _, rule := range rules {
		if pe.Enforce(rule, *policyContext) {
			matchedRules = append(matchedRules, rule)
		}
	}

	if len(matchedRules) == 0 {
		// authenticated but not authorized
		h := req.Header
		username := h.Get("X-User-Name")
		userDomain := h.Get("X-User-Domain-Name")
		scopedDomain := h.Get("X-Domain-Name")
		scopedProject := h.Get("X-Project-Name")
		scopedProjectDomain := h.Get("X-Project-Domain-Name")
		scope := scopedProject + " in domain " + scopedProjectDomain
		if scopedProject == "" {
			scope = scopedDomain
		}
		actRoles := h.Get("X-Roles")
		reqRoles := viper.GetString("keystone.roles")
		http.Error(w, html.EscapeString(fmt.Sprintf("User %s@%s does not have monitoring permissions on %s (actual roles: %s, required roles: %s)", username, userDomain, scope, actRoles, reqRoles)), http.StatusForbidden)

		return false
	}

	// set cookie
	setAuthCookies(req, w)

	return true
}

func requestReauthentication(w http.ResponseWriter) {
	logg.Debug("expire cookie and request username/password input")
	http.SetCookie(w, &http.Cookie{
		Name:     authTokenCookieName,
		Path:     "/",
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("WWW-Authenticate", "Basic")
}

func setAuthCookies(req *http.Request, w http.ResponseWriter) {
	token := req.Header.Get(authTokenHeader)
	if token == "" {
		logg.Info("WARNING: X-Auth-Token Header is empty!?")
		return
	}
	logg.Debug("Setting cookie: %s...", token[1:len(token)/4])
	expiryStr := req.Header.Get(authTokenExpiryHeader)
	expiry, pErr := time.Parse(time.RFC3339Nano, expiryStr)
	if pErr != nil {
		logg.Info("WARNING: Incompatible token format for expiry data: %s", expiryStr)
		expiry = time.Now().UTC().Add(viper.GetDuration("keystone.token_cache_time"))
	}
	// set token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     authTokenCookieName,
		Path:     "/",
		Value:    token,
		Expires:  expiry.UTC(),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	// remember domain as cookie so that reauthentication during Prometheus API calls (no domain prefix)
	// works with plain username and password
	http.SetCookie(w, &http.Cookie{
		Name:     userDomainCookieName,
		Path:     "/",
		Value:    req.Header.Get(userDomainHeader),
		MaxAge:   60 * 60 * 24,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func authorize(wrappedHandlerFunc func(w http.ResponseWriter, req *http.Request),
	guessScope bool, rule string) func(w http.ResponseWriter, req *http.Request) {

	return func(w http.ResponseWriter, req *http.Request) {
		// Get keystone from context (secure, race-condition-free approach)
		ks := getKeystoneFromContext(req.Context())
		if ks == nil {
			// Context-based keystone resolution is mandatory for security
			// All requests must go through keystoneResolutionMiddleware
			http.Error(w, "Internal server error: keystone context not available", http.StatusInternalServerError)
			logg.Error("Missing keystone context - request may have bypassed keystoneResolutionMiddleware")
			return
		}
		if authorizeRules(ks, w, req, guessScope, []string{rule}) {
			wrappedHandlerFunc(w, req)
		}
	}
}

// keystoneResolutionMiddleware determines keystone type early and consistently
// This middleware eliminates race conditions by resolving keystone selection
// once at the beginning of request processing and storing it in request context.
func keystoneResolutionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Determine keystone type early and consistently
		keystoneType, keystoneDriver := determineKeystoneForRequest(r)

		// Set both type and instance in request context
		ctx := context.WithValue(r.Context(), keystoneTypeKey, keystoneType)
		ctx = context.WithValue(ctx, keystoneInstanceKey, keystoneDriver)

		logg.Debug("Request routed to %s keystone", keystoneType)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// determineKeystoneForRequest provides robust keystone determination with validation
// This function implements the core logic for selecting regional vs global keystone
// and includes proper error handling for invalid global flags.
func determineKeystoneForRequest(r *http.Request) (string, keystone.Driver) {
	// Parse global flag with proper validation
	isGlobal, err := parseGlobalRequest(r)
	if err != nil {
		logg.Error("Invalid global flag in request: %v", err)
		return "regional", keystoneInstance // Fallback to regional
	}

	if isGlobal && globalKeystoneInstance != nil {
		return "global", globalKeystoneInstance
	}

	return "regional", keystoneInstance
}

// parseGlobalRequest handles robust boolean parsing with multiple formats
// It supports both URL parameters and headers with proper precedence.
func parseGlobalRequest(r *http.Request) (bool, error) {
	// Precedence: URL parameter > Header > default false
	if param := r.URL.Query().Get("global"); param != "" {
		return parseBoolean(param, "global parameter")
	}

	if header := r.Header.Get("X-Global-Region"); header != "" {
		return parseBoolean(header, "X-Global-Region header")
	}

	return false, nil
}

// parseBoolean provides robust boolean parsing with multiple accepted formats
func parseBoolean(value, source string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off", "":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value in %s: '%s'", source, value)
	}
}

// getKeystoneFromContext retrieves the keystone instance from request context
// This is the secure, thread-safe way to get keystone instances throughout request processing.
func getKeystoneFromContext(ctx context.Context) keystone.Driver {
	if driver, ok := ctx.Value(keystoneInstanceKey).(keystone.Driver); ok {
		return driver
	}
	// Return nil to indicate context-based keystone not available
	// Caller should handle fallback logic
	return nil
}

// getKeystoneTypeFromContext retrieves the keystone type from request context
func getKeystoneTypeFromContext(ctx context.Context) string {
	if keystoneType, ok := ctx.Value(keystoneTypeKey).(string); ok {
		return keystoneType
	}
	return "regional"
}

func gaugeInflight(handler http.Handler) http.Handler {
	inflightGauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "maia_requests_inflight", Help: "Number of inflight HTTP requests served by Maia"})
	prometheus.MustRegister(inflightGauge)

	return promhttp.InstrumentHandlerInFlight(inflightGauge, handler)
}

func observeDuration(handlerFunc http.HandlerFunc, handler string) http.HandlerFunc {
	durationSummary := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{Name: "maia_request_duration_seconds", Help: "Duration/latency of a Maia request", ConstLabels: prometheus.Labels{"handler": handler}}, nil)
	prometheus.MustRegister(durationSummary)

	return promhttp.InstrumentHandlerDuration(durationSummary, handlerFunc)
}

func observeResponseSize(handlerFunc http.HandlerFunc, handler string) http.HandlerFunc {
	durationSummary := prometheus.NewSummaryVec(prometheus.SummaryOpts{Name: "maia_response_size_bytes", Help: "Size of the Maia response (e.g. to a query)", ConstLabels: prometheus.Labels{"handler": handler}}, nil)
	prometheus.MustRegister(durationSummary)

	return promhttp.InstrumentHandlerResponseSize(durationSummary, handlerFunc).ServeHTTP
}
