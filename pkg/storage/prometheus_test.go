// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"net/http"
	"testing"

	"github.com/h2non/gock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

const (
	prometheusURL = "http://thanos.local/thanos"
	federateURL   = "http://prometheus.local"
)

func setupTest(t *testing.T) Driver { //nolint:unparam
	// load test policy (where everything is allowed)
	viper.Set("maia.storage_driver", "prometheus")
	viper.Set("maia.label_value_ttl", "72h")
	viper.Set("maia.prometheus_url", prometheusURL)
	viper.Set("maia.federate_url", federateURL)

	return NewPrometheusDriver(prometheusURL, map[string]string{})
}

func mocksToStrings(mocks []gock.Mock) []string {
	s := make([]string, len(mocks))
	for i, m := range mocks {
		r := m.Request()
		s[i] = r.Method + " " + r.URLStruct.String()
	}
	return s
}

func TestNewPrometheusDriver(t *testing.T) {
	defer gock.Off()

	setupTest(t)

	assertDone(t)
}
func assertDone(t *testing.T) bool { //nolint:unparam
	return assert.True(t, gock.IsDone(), "pending mocks: %v\nunmatched requests: %v", mocksToStrings(gock.Pending()), gock.GetUnmatchedRequests())
}

func TestFederate(t *testing.T) {
	defer gock.Off()

	ps := setupTest(t)

	gock.New(federateURL).Get("/federate").
		MatchParams(map[string]string{"match[]": "{vmware_name=\"win_cifs_13\",project_id=\"p00001\"}"}).
		Reply(http.StatusOK).
		File("fixtures/federate.txt").
		AddHeader("Content-Type", PlainText)

	_, err := ps.Federate([]string{"{vmware_name=\"win_cifs_13\",project_id=\"p00001\"}"}, PlainText)

	assert.Nil(t, err, "Federate should not fail")

	assertDone(t)
}

func TestLabelValues(t *testing.T) {
	defer gock.Off()

	ps := setupTest(t)

	gock.New(prometheusURL).Get("/api/v1/label/service/values").
		Reply(http.StatusOK).
		File("fixtures/label_values.json").
		AddHeader("Content-Type", JSON)

	_, err := ps.LabelValues("service", JSON)

	assert.Nil(t, err, "label/.../values should not fail")

	assertDone(t)
}

// TestLabels I tried to match this similarly to TestLabelValues
// It passes, but I can't seem to sort out if it actually works.
// It feels like it's not actually using the fixture, but I'm not sure.
func TestLabels(t *testing.T) {
	defer gock.Off()

	ps := setupTest(t)

	// Mock the labels endpoint
	gock.New(prometheusURL).Get("/api/v1/labels").
		Reply(http.StatusOK).
		File("fixtures/label_names.json").
		AddHeader("Content-Type", JSON)

	start := "2023-05-12T00:00:00Z"
	end := "2023-05-12T23:59:59Z"
	match := []string{"project_id=\"p00001\""}
	_, err := ps.Labels(start, end, match, JSON)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, err, "labels should not fail")

	assertDone(t)
}

// TestValidateUpstreamURL exercises the SSRF defense-in-depth check that backs
// CodeQL go/request-forgery. mapURL() rewrites Host/Scheme/User to the trusted
// upstream, so the validator should accept the configured Prometheus host and
// the federate host but reject anything else.
func TestValidateUpstreamURL(t *testing.T) {
	setupTest(t)
	driver := NewPrometheusDriver(prometheusURL, map[string]string{}).(*prometheusStorageClient)

	cases := []struct {
		name    string
		url     string
		wantErr string // substring match; empty = expect success
	}{
		{"trusted upstream", "http://thanos.local/thanos/api/v1/query", ""},
		{"trusted federate", "http://prometheus.local/federate", ""},
		{"foreign host rejected", "http://evil.example.com/api/v1/query", "untrusted host"},
		{"localhost rejected", "http://127.0.0.1:9090/api/v1/query", "untrusted host"},
		{"empty host rejected", "http:///api/v1/query", "empty host"},
		{"unknown scheme rejected", "file:///etc/passwd", "scheme"},
		{"ftp scheme rejected", "ftp://thanos.local/", "scheme"},
		{"malformed url rejected", "http://%zz", "invalid URL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := driver.validateUpstreamURL(tc.url)
			if tc.wantErr == "" {
				assert.NoError(t, err, "expected %q to be accepted", tc.url)
				return
			}
			assert.Error(t, err, "expected %q to be rejected", tc.url)
			if err != nil {
				assert.Contains(t, err.Error(), tc.wantErr, "wrong rejection reason for %q", tc.url)
			}
		})
	}
}

// TestSendToPrometheusRejectsUntrustedHost ensures the validation actually
// blocks an outbound request, not just returns an error from a helper.
func TestSendToPrometheusRejectsUntrustedHost(t *testing.T) {
	defer gock.Off()
	setupTest(t)
	driver := NewPrometheusDriver(prometheusURL, map[string]string{}).(*prometheusStorageClient)

	// No gock mock registered — if validation fails to block, the request would
	// hit the real network (or gock's "unmatched" path) and we'd see a different
	// error. Validation must short-circuit before any HTTP call.
	resp, err := driver.sendToPrometheus("GET", "http://attacker.invalid/api/v1/query", nil, nil)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "untrusted host")
}
