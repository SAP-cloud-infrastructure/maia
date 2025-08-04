// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	policy "github.com/databus23/goslo.policy"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"

	"github.com/sapcc/maia/pkg/keystone"
)

// Mock keystone driver for testing
type mockKeystoneDriver struct {
	name string
}

func (m *mockKeystoneDriver) AuthenticateRequest(ctx context.Context, req *http.Request, guessScope bool) (*policy.Context, keystone.AuthenticationError) {
	return nil, nil
}

func (m *mockKeystoneDriver) Authenticate(ctx context.Context, options gophercloud.AuthOptions) (*policy.Context, string, keystone.AuthenticationError) {
	return nil, "", nil
}

func (m *mockKeystoneDriver) ChildProjects(ctx context.Context, projectID string) ([]string, error) {
	return []string{}, nil
}

func (m *mockKeystoneDriver) UserProjects(ctx context.Context, userID string) ([]tokens.Scope, error) {
	return []tokens.Scope{}, nil
}

func (m *mockKeystoneDriver) ServiceURL() string {
	return "http://test-" + m.name + ".example.com"
}

func TestEarlyKeystoneResolution(t *testing.T) {
	// Setup mock keystone instances
	regionalKeystone := &mockKeystoneDriver{name: "regional"}
	globalKeystone := &mockKeystoneDriver{name: "global"}

	// Set up the global instances for testing
	originalRegional := keystoneInstance
	originalGlobal := globalKeystoneInstance
	defer func() {
		keystoneInstance = originalRegional
		globalKeystoneInstance = originalGlobal
	}()

	keystoneInstance = regionalKeystone
	globalKeystoneInstance = globalKeystone

	testCases := []struct {
		name           string
		url            string
		headers        map[string]string
		expectedType   string
		expectedDriver keystone.Driver
	}{
		{
			name:           "Regional request",
			url:            "/api/v1/query",
			headers:        nil,
			expectedType:   "regional",
			expectedDriver: regionalKeystone,
		},
		{
			name:           "Global param",
			url:            "/api/v1/query?global=true",
			headers:        nil,
			expectedType:   "global",
			expectedDriver: globalKeystone,
		},
		{
			name:           "Global header",
			url:            "/api/v1/query",
			headers:        map[string]string{"X-Global-Region": "true"},
			expectedType:   "global",
			expectedDriver: globalKeystone,
		},
		{
			name:           "Param precedence over header",
			url:            "/api/v1/query?global=false",
			headers:        map[string]string{"X-Global-Region": "true"},
			expectedType:   "regional",
			expectedDriver: regionalKeystone,
		},
		{
			name:           "Global param with various boolean formats",
			url:            "/api/v1/query?global=1",
			headers:        nil,
			expectedType:   "global",
			expectedDriver: globalKeystone,
		},
		{
			name:           "Global param yes format",
			url:            "/api/v1/query?global=yes",
			headers:        nil,
			expectedType:   "global",
			expectedDriver: globalKeystone,
		},
		{
			name:           "Global param on format",
			url:            "/api/v1/query?global=on",
			headers:        nil,
			expectedType:   "global",
			expectedDriver: globalKeystone,
		},
		{
			name:           "Global param false formats",
			url:            "/api/v1/query?global=0",
			headers:        nil,
			expectedType:   "regional",
			expectedDriver: regionalKeystone,
		},
		{
			name:           "Global param no format",
			url:            "/api/v1/query?global=no",
			headers:        nil,
			expectedType:   "regional",
			expectedDriver: regionalKeystone,
		},
		{
			name:           "Global param off format",
			url:            "/api/v1/query?global=off",
			headers:        nil,
			expectedType:   "regional",
			expectedDriver: regionalKeystone,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, http.NoBody)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			keystoneType, keystoneDriver := determineKeystoneForRequest(req)

			if keystoneType != tc.expectedType {
				t.Errorf("Expected keystone type %s, got %s", tc.expectedType, keystoneType)
			}

			if keystoneDriver != tc.expectedDriver {
				t.Errorf("Expected keystone driver %v, got %v", tc.expectedDriver, keystoneDriver)
			}
		})
	}
}

func TestInvalidBooleanHandling(t *testing.T) {
	testCases := []struct {
		name         string
		url          string
		headers      map[string]string
		expectedType string // Should fallback to regional on error
	}{
		{
			name:         "Invalid global param",
			url:          "/api/v1/query?global=invalid",
			headers:      nil,
			expectedType: "regional",
		},
		{
			name:         "Invalid global header",
			url:          "/api/v1/query",
			headers:      map[string]string{"X-Global-Region": "invalid"},
			expectedType: "regional",
		},
		{
			name:         "Empty global param should be false",
			url:          "/api/v1/query?global=",
			headers:      nil,
			expectedType: "regional",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, http.NoBody)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			keystoneType, _ := determineKeystoneForRequest(req)

			if keystoneType != tc.expectedType {
				t.Errorf("Expected keystone type %s on error, got %s", tc.expectedType, keystoneType)
			}
		})
	}
}

func TestKeystoneResolutionMiddleware(t *testing.T) {
	// Setup mock keystone instances
	regionalKeystone := &mockKeystoneDriver{name: "regional"}
	globalKeystone := &mockKeystoneDriver{name: "global"}

	// Set up the global instances for testing
	originalRegional := keystoneInstance
	originalGlobal := globalKeystoneInstance
	defer func() {
		keystoneInstance = originalRegional
		globalKeystoneInstance = originalGlobal
	}()

	keystoneInstance = regionalKeystone
	globalKeystoneInstance = globalKeystone

	// Test handler that verifies context contains keystone info
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify keystone instance is in context
		driver := getKeystoneFromContext(r.Context())
		if driver == nil {
			t.Error("Expected keystone driver in context, got nil")
			return
		}

		// Verify keystone type is in context
		keystoneType := getKeystoneTypeFromContext(r.Context())
		if keystoneType == "" {
			t.Error("Expected keystone type in context, got empty string")
			return
		}

		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	})

	// Wrap with middleware
	middlewareHandler := keystoneResolutionMiddleware(testHandler)

	testCases := []struct {
		name    string
		url     string
		headers map[string]string
	}{
		{
			name:    "Regional request",
			url:     "/api/v1/query",
			headers: nil,
		},
		{
			name:    "Global request via param",
			url:     "/api/v1/query?global=true",
			headers: nil,
		},
		{
			name:    "Global request via header",
			url:     "/api/v1/query",
			headers: map[string]string{"X-Global-Region": "true"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, http.NoBody)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			recorder := httptest.NewRecorder()
			middlewareHandler.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, recorder.Code)
			}
		})
	}
}

func TestRequestContextConsistency(t *testing.T) {
	// Setup mock keystone instances
	regionalKeystone := &mockKeystoneDriver{name: "regional"}
	globalKeystone := &mockKeystoneDriver{name: "global"}

	// Set up the global instances for testing
	originalRegional := keystoneInstance
	originalGlobal := globalKeystoneInstance
	defer func() {
		keystoneInstance = originalRegional
		globalKeystoneInstance = originalGlobal
	}()

	keystoneInstance = regionalKeystone
	globalKeystoneInstance = globalKeystone

	// Test that keystone instance remains consistent throughout request lifecycle
	var firstKeystoneInstance keystone.Driver
	var secondKeystoneInstance keystone.Driver

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First call to get keystone from context
		firstKeystoneInstance = getKeystoneFromContext(r.Context())

		// Simulate middleware or handler calling getKeystoneFromContext again
		secondKeystoneInstance = getKeystoneFromContext(r.Context())

		w.WriteHeader(http.StatusOK)
	})

	middlewareHandler := keystoneResolutionMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/query?global=true", http.NoBody)
	recorder := httptest.NewRecorder()
	middlewareHandler.ServeHTTP(recorder, req)

	// Verify both calls returned the same instance
	if firstKeystoneInstance != secondKeystoneInstance {
		t.Error("Keystone instance changed during request processing - race condition detected!")
	}

	// Verify we got the expected global instance
	if firstKeystoneInstance.ServiceURL() != globalKeystone.ServiceURL() {
		t.Error("Expected global keystone instance, got different instance")
	}
}

func TestParseBooleanFormats(t *testing.T) {
	testCases := []struct {
		value    string
		expected bool
		hasError bool
	}{
		// True values
		{"true", true, false},
		{"TRUE", true, false},
		{"True", true, false},
		{"1", true, false},
		{"yes", true, false},
		{"YES", true, false},
		{"on", true, false},
		{"ON", true, false},

		// False values
		{"false", false, false},
		{"FALSE", false, false},
		{"False", false, false},
		{"0", false, false},
		{"no", false, false},
		{"NO", false, false},
		{"off", false, false},
		{"OFF", false, false},
		{"", false, false}, // Empty string should be false

		// Values with whitespace (should be trimmed)
		{" true ", true, false},
		{" false ", false, false},

		// Invalid values
		{"invalid", false, true},
		{"maybe", false, true},
		{"2", false, true},
	}

	for _, tc := range testCases {
		t.Run("value_"+tc.value, func(t *testing.T) {
			result, err := parseBoolean(tc.value, "test")

			if tc.hasError {
				if err == nil {
					t.Errorf("Expected error for value '%s', got none", tc.value)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for value '%s', got: %v", tc.value, err)
				}
				if result != tc.expected {
					t.Errorf("Expected %v for value '%s', got %v", tc.expected, tc.value, result)
				}
			}
		})
	}
}

func TestSecureKeystoneContext(t *testing.T) {
	// Setup mock keystone instances
	regionalKeystone := &mockKeystoneDriver{name: "regional"}
	globalKeystone := &mockKeystoneDriver{name: "global"}

	// Set up the global instances for testing
	originalRegional := keystoneInstance
	originalGlobal := globalKeystoneInstance
	defer func() {
		keystoneInstance = originalRegional
		globalKeystoneInstance = originalGlobal
	}()

	keystoneInstance = regionalKeystone
	globalKeystoneInstance = globalKeystone

	// Test that context-based keystone selection is mandatory
	req := httptest.NewRequest(http.MethodGet, "/api/v1/query?global=true", http.NoBody)

	// Without context, should return nil (no fallback to insecure methods)
	result := getKeystoneFromContext(req.Context())
	if result != nil {
		t.Error("Should return nil when no keystone context is available")
	}

	// Test that context-based keystone selection works
	ctx := context.WithValue(req.Context(), keystoneInstanceKey, globalKeystone)
	reqWithContext := req.WithContext(ctx)

	result = getKeystoneFromContext(reqWithContext.Context())

	// Should use context value
	if result.ServiceURL() != globalKeystone.ServiceURL() {
		t.Error("Context-based keystone selection not working")
	}
}
