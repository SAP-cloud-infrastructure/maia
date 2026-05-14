// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"errors"

	policy "github.com/databus23/goslo.policy"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/SAP-cloud-infrastructure/maia/pkg/keystone"
	"github.com/SAP-cloud-infrastructure/maia/pkg/storage"
	"github.com/SAP-cloud-infrastructure/maia/pkg/test"
)

var projectContext = &policy.Context{Request: map[string]string{"project_id": "12345", "domain_id": "77777", "user_id": "u12345"},
	Auth: map[string]string{"project_id": "12345", "project_name": "testproject",
		"project_domain_name": "testdomain", "project_domain_id": "77777",
		"user_id": "u12345", "user_name": "testuser", "user_domain_name": "testdomain", "user_domain_id": "77777"},
	Roles: []string{"monitoring_viewer"}}
var projectInsufficientRolesContext = &policy.Context{Request: map[string]string{"project_id": "12345", "domain_id": "77777", "user_id": "u12345"},
	Auth: map[string]string{"project_id": "12345", "project_name": "testproject",
		"project_domain_name": "testdomain", "project_domain_id": "77777",
		"user_id": "u12345", "user_name": "testuser", "user_domain_name": "testdomain", "user_domain_id": "77777"},
	Roles: []string{"member"}}
var projectHeader = map[string]string{"X-User-Id": projectContext.Auth["user_id"], "X-User-Name": projectContext.Auth["user_name"],
	"X-User-Domain-Name": projectContext.Auth["user_domain_name"],
	"X-Project-Id":       projectContext.Auth["project_id"], "X-Project-Name": projectContext.Auth["project_name"]}
var domainContext = &policy.Context{Request: map[string]string{"project_id": "12345", "domain_id": "77777", "user_id": "u12345"},
	Auth: map[string]string{"domain_id": "77777", "domain_name": "testdomain",
		"user_id": "u12345", "user_name": "testuser", "user_domain_name": "testdomain", "user_domain_id": "77777"},
	Roles: []string{"monitoring_viewer"}}
var domainHeader = map[string]string{"X-User-Id": domainContext.Auth["user_id"], "X-User-Name": domainContext.Auth["user_name"],
	"X-User-Domain-Name": domainContext.Auth["user_domain_name"],
	"X-Domain-Id":        domainContext.Auth["domain_id"], "X-Domain-Name": domainContext.Auth["domain_name"]}

func setupTest(t *testing.T, controller *gomock.Controller) (router http.Handler, keystoneDriver *keystone.MockDriver, storageDriver *storage.MockDriver) { //nolint:unparam
	// load test policy (where everything is allowed)
	viper.Set("keystone.policy_file", "../test/policy.json")
	viper.Set("maia.label_value_ttl", "72h")

	// create test driver with the domains and projects from start-data.sql
	keystoneDriver = keystone.NewMockDriver(controller)
	storageDriver = storage.NewMockDriver(controller)

	prometheus.DefaultRegisterer = prometheus.NewPedanticRegistry()

	// Pass nil as globalKeystoneDriver for tests that don't need it
	router = setupRouter(keystoneDriver, nil, storageDriver)

	return router, keystoneDriver, storageDriver
}

func expectAuthByProjectID(keystoneMock *keystone.MockDriver) {
	httpReqMatcher := test.HTTPRequestMatcher{InjectHeader: projectHeader}
	authCall := keystoneMock.EXPECT().AuthenticateRequest(test.MatchContext(), httpReqMatcher, false).Return(projectContext, nil)
	keystoneMock.EXPECT().ChildProjects(test.MatchContext(), projectContext.Auth["project_id"]).Return([]string{}, nil).After(authCall)
}

func expectAuthByDomainName(keystoneMock *keystone.MockDriver) {
	httpReqMatcher := test.HTTPRequestMatcher{InjectHeader: domainHeader}
	keystoneMock.EXPECT().AuthenticateRequest(test.MatchContext(), httpReqMatcher, false).Return(domainContext, nil)
}

func expectAuthWithChildren(keystoneMock *keystone.MockDriver) {
	httpReqMatcher := test.HTTPRequestMatcher{InjectHeader: projectHeader}
	authCall := keystoneMock.EXPECT().AuthenticateRequest(test.MatchContext(), httpReqMatcher, false).Return(projectContext, nil)
	keystoneMock.EXPECT().ChildProjects(test.MatchContext(), projectContext.Auth["project_id"]).Return([]string{"67890"}, nil).After(authCall)
}

func expectAuthByDefaults(keystoneMock *keystone.MockDriver) {
	httpReqMatcher := test.HTTPRequestMatcher{InjectHeader: projectHeader}
	authCall := keystoneMock.EXPECT().AuthenticateRequest(test.MatchContext(), httpReqMatcher, true).Return(projectContext, nil)
	keystoneMock.EXPECT().UserProjects(test.MatchContext(), projectContext.Auth["user_id"]).Return([]tokens.Scope{{ProjectID: projectContext.Auth["project_id"], DomainID: projectContext.Auth["project_domain_id"]}}, nil).After(authCall)
}

func expectAuthAndFail(keystoneMock *keystone.MockDriver) {
	httpReqMatcher := test.HTTPRequestMatcher{InjectHeader: projectHeader}
	keystoneMock.EXPECT().AuthenticateRequest(test.MatchContext(), httpReqMatcher, false).Return(nil, keystone.NewAuthenticationError(keystone.StatusWrongCredentials, "negativetesterror"))
}

func expectPlainBasicAuthAndFail(keystoneMock *keystone.MockDriver) {
	httpReqMatcher := test.HTTPRequestMatcher{InjectHeader: projectHeader}
	keystoneMock.EXPECT().AuthenticateRequest(test.MatchContext(), httpReqMatcher, true).Return(nil, keystone.NewAuthenticationError(keystone.StatusWrongCredentials, "negativetesterror"))
}

func expectAuthAndDenyAuthorization(keystoneMock *keystone.MockDriver) {
	httpReqMatcher := test.HTTPRequestMatcher{InjectHeader: projectHeader}
	keystoneMock.EXPECT().AuthenticateRequest(test.MatchContext(), httpReqMatcher, false).Return(projectInsufficientRolesContext, nil)
}

// HTTP based tests

func TestFederate(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, storageMock := setupTest(t, ctrl)

	expectAuthByDomainName(keystoneMock)
	storageMock.EXPECT().Federate([]string{"{vmware_name=\"win_cifs_13\",domain_id=\"77777\"}"}, storage.PlainText).Return(test.HTTPResponseFromFile("fixtures/federate.txt"), nil)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic u12345|@77777:password")), "Accept": storage.PlainText},
		Method:           "GET",
		Path:             "/federate?match[]={vmware_name=%22win_cifs_13%22}",
		ExpectStatusCode: http.StatusOK,
		ExpectFile:       "fixtures/federate.txt",
	}.Check(t, router)
}

func TestFederate_errorNoMatch(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	expectAuthByDomainName(keystoneMock)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic u12345|@77777:password")), "Accept": storage.PlainText},
		Method:           "GET",
		Path:             "/federate?bla[]={vmwa...}",
		ExpectStatusCode: http.StatusBadRequest,
	}.Check(t, router)
}

func TestFederate_errorInvalidSelector(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	expectAuthByDomainName(keystoneMock)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic u12345|@77777:password")), "Accept": storage.PlainText},
		Method:           "GET",
		Path:             "/federate?match[]={invalid_syntax=}",
		ExpectStatusCode: http.StatusBadRequest,
	}.Check(t, router)
}

func TestFederate_errorBackendFailed(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, storageMock := setupTest(t, ctrl)

	expectAuthByDomainName(keystoneMock)
	storageMock.EXPECT().Federate([]string{"{vmware_name=\"win_cifs_13\",domain_id=\"77777\"}"}, storage.PlainText).Return(nil, errors.New("testerror"))

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic u12345|@77777:password")), "Accept": storage.PlainText},
		Method:           "GET",
		Path:             "/federate?match[]={vmware_name=%22win_cifs_13%22}",
		ExpectStatusCode: http.StatusServiceUnavailable,
	}.Check(t, router)
}

func TestSeries(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, storageMock := setupTest(t, ctrl)

	expectAuthWithChildren(keystoneMock)
	storageMock.EXPECT().Series([]string{"{component!=\"\",project_id=~\"12345|67890\"}"}, "2017-07-01T20:10:30.781Z", "2017-07-02T04:00:00.000Z", storage.JSON).Return(test.HTTPResponseFromFile("fixtures/series.json"), nil)

	test.APIRequest{
		Headers:          map[string]string{"X-Auth-Token": "someverylongtokenideed", "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/api/v1/series?match[]={component!=%22%22}&end=2017-07-02T04:00:00.000Z&start=2017-07-01T20:10:30.781Z",
		ExpectStatusCode: http.StatusOK,
		ExpectJSON:       "fixtures/series.json",
	}.Check(t, router)
}

func TestSeries_failAuthentication(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	expectAuthAndFail(keystoneMock)

	test.APIRequest{
		Method:           "GET",
		Path:             "/api/v1/series?match[]={component!=%22%22}&end=2017-07-02T04:00:00.000Z&start=2017-07-01T20:10:30.781Z",
		ExpectStatusCode: http.StatusUnauthorized,
	}.Check(t, router)
}

func TestSeries_failAuthorization(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	expectAuthAndDenyAuthorization(keystoneMock)

	test.APIRequest{
		Method:           "GET",
		Path:             "/api/v1/series?match[]={component!=%22%22}&end=2017-07-02T04:00:00.000Z&start=2017-07-01T20:10:30.781Z",
		ExpectStatusCode: http.StatusForbidden,
	}.Check(t, router)
}

func TestLabels(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, storageMock := setupTest(t, ctrl)

	expectAuthWithChildren(keystoneMock)
	storageMock.EXPECT().Labels(
		"2017-07-01T20:10:30.781Z",
		"2017-07-02T04:00:00.000Z",
		[]string{"{component!=\"\",project_id=~\"12345|67890\"}"},
		storage.JSON,
	).Return(test.HTTPResponseFromFile("fixtures/labels.json"), nil)

	test.APIRequest{
		Headers:          map[string]string{"X-Auth-Token": "someverylongtokenideed", "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/api/v1/labels?match[]={component!=%22%22}&end=2017-07-02T04:00:00.000Z&start=2017-07-01T20:10:30.781Z",
		ExpectStatusCode: http.StatusOK,
		ExpectJSON:       "fixtures/labels.json",
	}.Check(t, router)
}

func TestLabels_domainScope(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, storageMock := setupTest(t, ctrl)

	expectAuthByDomainName(keystoneMock)
	storageMock.EXPECT().Labels(
		"2017-07-01T20:10:30.781Z",
		"2017-07-02T04:00:00.000Z",
		[]string{"{component!=\"\",domain_id=\"77777\"}"},
		storage.JSON,
	).Return(test.HTTPResponseFromFile("fixtures/labels.json"), nil)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic u12345|@77777:password")), "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/api/v1/labels?match[]={component!=%22%22}&end=2017-07-02T04:00:00.000Z&start=2017-07-01T20:10:30.781Z",
		ExpectStatusCode: http.StatusOK,
		ExpectJSON:       "fixtures/labels.json",
	}.Check(t, router)
}

func TestLabels_errorNoMatch(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	expectAuthByProjectID(keystoneMock)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic user_id|12345:password")), "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/api/v1/labels",
		ExpectStatusCode: http.StatusBadRequest,
	}.Check(t, router)
}

func TestLabels_errorInvalidSelector(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	expectAuthByProjectID(keystoneMock)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic user_id|12345:password")), "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/api/v1/labels?match[]={invalid_syntax=}",
		ExpectStatusCode: http.StatusBadRequest,
	}.Check(t, router)
}

func TestLabels_errorBackendFailed(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, storageMock := setupTest(t, ctrl)

	expectAuthWithChildren(keystoneMock)
	storageMock.EXPECT().Labels(
		"2017-07-01T20:10:30.781Z",
		"2017-07-02T04:00:00.000Z",
		[]string{"{component!=\"\",project_id=~\"12345|67890\"}"},
		storage.JSON,
	).Return(nil, errors.New("testerror"))

	test.APIRequest{
		Headers:          map[string]string{"X-Auth-Token": "someverylongtokenideed", "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/api/v1/labels?match[]={component!=%22%22}&end=2017-07-02T04:00:00.000Z&start=2017-07-01T20:10:30.781Z",
		ExpectStatusCode: http.StatusServiceUnavailable,
	}.Check(t, router)
}

func TestLabels_failAuthentication(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	expectAuthAndFail(keystoneMock)

	test.APIRequest{
		Method:           "GET",
		Path:             "/api/v1/labels?match[]={component!=%22%22}&end=2017-07-02T04:00:00.000Z&start=2017-07-01T20:10:30.781Z",
		ExpectStatusCode: http.StatusUnauthorized,
	}.Check(t, router)
}

func TestLabels_failAuthorization(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	expectAuthAndDenyAuthorization(keystoneMock)

	test.APIRequest{
		Method:           "GET",
		Path:             "/api/v1/labels?match[]={component!=%22%22}&end=2017-07-02T04:00:00.000Z&start=2017-07-01T20:10:30.781Z",
		ExpectStatusCode: http.StatusForbidden,
	}.Check(t, router)
}

func TestLabelValues(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, storageMock := setupTest(t, ctrl)

	expectAuthByProjectID(keystoneMock)
	// Maia's label-values implementation uses the series API and a time-based filter stale series out. The exact start
	// and end date of the filter cannot be predicted, therefore we accept anything that is a parsable date.
	storageMock.EXPECT().QueryRange("count by (service) ({project_id=\"12345\",service!=\"\"})", test.TimeStringMatcher{}, test.TimeStringMatcher{}, viper.Get("maia.label_value_ttl"), "", storage.JSON).Return(test.HTTPResponseFromFile("fixtures/label_values_query_range.json"), nil)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic user_id|12345:password")), "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/api/v1/label/service/values",
		ExpectStatusCode: http.StatusOK,
		ExpectJSON:       "fixtures/label_values.json",
	}.Check(t, router)
}

// TestLabelValues_errorNonMatrixResult verifies that LabelValues returns a 502
// error (not a panic) when the backing Prometheus query_range returns a
// non-matrix result type (e.g. a vector). Before the fix, the bare type
// assertion sr.Data.Value.(model.Matrix) in LabelValues would panic for any
// non-matrix Value.
func TestLabelValues_errorNonMatrixResult(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, storageMock := setupTest(t, ctrl)

	expectAuthByProjectID(keystoneMock)
	storageMock.EXPECT().QueryRange(
		"count by (service) ({project_id=\"12345\",service!=\"\"})",
		test.TimeStringMatcher{}, test.TimeStringMatcher{},
		viper.Get("maia.label_value_ttl"), "", storage.JSON,
	).Return(test.HTTPResponseFromFile("fixtures/label_values_query_range_vector.json"), nil)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic user_id|12345:password")), "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/api/v1/label/service/values",
		ExpectStatusCode: http.StatusBadGateway,
		ExpectJSON:       "fixtures/label_values_nonmatrix_error.json",
	}.Check(t, router)
}

func TestQuery(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, storageMock := setupTest(t, ctrl)

	expectAuthByProjectID(keystoneMock)
	storageMock.EXPECT().Query("sum(blackbox_api_status_gauge{check=~\"keystone\",project_id=\"12345\"})", "2017-07-01T20:10:30.781Z", "24m", storage.JSON).Return(test.HTTPResponseFromFile("fixtures/query.json"), nil)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic user_id|12345:password")), "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/api/v1/query?query=sum(blackbox_api_status_gauge{check%3D~%22keystone%22})&time=2017-07-01T20:10:30.781Z&timeout=24m",
		ExpectStatusCode: http.StatusOK,
		ExpectJSON:       "fixtures/query.json",
	}.Check(t, router)
}

func TestQuery_syntaxError(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	expectAuthByProjectID(keystoneMock)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic user_id|12345:password")), "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/api/v1/query?query=sum(blackbox_api_status_gauge{check%3D~%22keystone%22}&time=2017-07-01T20:10:30.781Z&timeout=24m",
		ExpectStatusCode: 400,
		ExpectJSON:       "fixtures/query_syntax_error.json",
	}.Check(t, router)
}

func TestQueryRange(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, storageMock := setupTest(t, ctrl)

	expectAuthByProjectID(keystoneMock)
	storageMock.EXPECT().QueryRange("sum({__name__=\"blackbox_api_status_gauge\",check=~\"keystone\",project_id=\"12345\"})", "2017-07-01T20:10:30.781Z", "2017-07-02T04:00:00.000Z", "5m", "90s", storage.JSON).Return(test.HTTPResponseFromFile("fixtures/query_range.json"), nil)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic user_id|12345:password")), "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/api/v1/query_range?query=sum(%7B__name__%3D%22blackbox_api_status_gauge%22%2Ccheck%3D~%22keystone%22%2Cproject_id%3D%2212345%22%7D)&end=2017-07-02T04:00:00.000Z&start=2017-07-01T20:10:30.781Z&step=5m&timeout=90s",
		ExpectStatusCode: http.StatusOK,
		ExpectJSON:       "fixtures/query_range.json",
	}.Check(t, router)
}

func TestAPIMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	keystoneMock.EXPECT().ServiceURL().Return("http://localhost:9091/api/v1")

	test.APIRequest{
		Method:           "GET",
		Path:             "/api",
		ExpectStatusCode: 300,
		ExpectJSON:       "fixtures/api-metadata.json",
	}.Check(t, router)
}

func TestServeStaticContent(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, _, _ := setupTest(t, ctrl)

	test.APIRequest{
		Method:           "GET",
		Path:             "/static/css/graph.css",
		ExpectStatusCode: http.StatusOK,
		ExpectFile:       "../../web/static/css/graph.css",
	}.Check(t, router)
}

func TestServeStaticContent_notFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, _, _ := setupTest(t, ctrl)

	test.APIRequest{
		Method:           "GET",
		Path:             "/static/bla.xyz",
		ExpectStatusCode: http.StatusNotFound,
	}.Check(t, router)
}

func TestGraph(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)
	expectAuthByDefaults(keystoneMock)

	test.APIRequest{
		Method:           "GET",
		Path:             "/testdomain/graph?project_id=" + projectContext.Auth["project_id"],
		ExpectStatusCode: http.StatusOK,
	}.Check(t, router)
}

func TestRoot_redirect(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, _, _ := setupTest(t, ctrl)

	test.APIRequest{
		Method:           "GET",
		Path:             "/" + projectContext.Auth["project_id"],
		ExpectStatusCode: http.StatusFound,
	}.Check(t, router)
}

func TestGraph_redirect(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, _, _ := setupTest(t, ctrl)

	test.APIRequest{
		Method:           "GET",
		Path:             "/graph?project_id=" + projectContext.Auth["project_id"],
		ExpectStatusCode: http.StatusFound,
	}.Check(t, router)
}

func TestGraph_otherOSDomain(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)
	expectPlainBasicAuthAndFail(keystoneMock)

	test.APIRequest{
		Headers:          map[string]string{"Authorization": base64.StdEncoding.EncodeToString([]byte("Basic testuser|12345:password")), "Accept": storage.JSON},
		Method:           "GET",
		Path:             "/nottestdomain/graph?project_id=" + projectContext.Auth["project_id"],
		ExpectStatusCode: http.StatusUnauthorized,
	}.Check(t, router)
}

func TestGlobalKeystoneRouting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Set policy file path - this is crucial
	viper.Set("keystone.policy_file", "../test/policy.json")
	viper.Set("maia.label_value_ttl", "72h")

	// Create mock keystones
	regularKeystone := keystone.NewMockDriver(ctrl)
	globalKeystone := keystone.NewMockDriver(ctrl)

	// Store instances for restoration
	originalKeystone := keystoneInstance
	originalGlobalKeystone := globalKeystoneInstance

	// Set global instances for testing
	keystoneInstance = regularKeystone
	globalKeystoneInstance = globalKeystone

	// Restore instances after test
	defer func() {
		keystoneInstance = originalKeystone
		globalKeystoneInstance = originalGlobalKeystone
	}()

	// Setup storage mock
	storageMock := storage.NewMockDriver(ctrl)

	// Reset prometheus registry to avoid conflicts
	prometheus.DefaultRegisterer = prometheus.NewPedanticRegistry()

	// Setup router with both keystones
	router := setupRouter(regularKeystone, globalKeystone, storageMock)

	// Test cases
	testCases := []struct {
		name           string
		path           string
		globalParam    string
		globalHeader   string
		expectedDriver *keystone.MockDriver
	}{
		{"Regular request", "/api/v1/query", "", "", regularKeystone},
		{"Global param request", "/api/v1/query", "true", "", globalKeystone},
		{"Global header request", "/api/v1/query", "", "true", globalKeystone},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create request with appropriate global params/headers
			url := tc.path
			if tc.globalParam != "" {
				url += "?global=" + tc.globalParam
			}
			req := httptest.NewRequest(http.MethodGet, url, http.NoBody)
			if tc.globalHeader != "" {
				req.Header.Set("X-Global-Region", tc.globalHeader)
			}

			// Set appropriate expectations on the expected driver
			tc.expectedDriver.EXPECT().AuthenticateRequest(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&policy.Context{}, nil)

			// Execute request
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
		})
	}
}

func TestRedirectPreservesGlobalFlag(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Store instances for restoration
	originalKeystone := keystoneInstance
	originalGlobalKeystone := globalKeystoneInstance

	// Create mock keystones
	regularKeystone := keystone.NewMockDriver(ctrl)
	globalKeystone := keystone.NewMockDriver(ctrl)

	// Set keystones
	keystoneInstance = regularKeystone
	globalKeystoneInstance = globalKeystone

	// Restore instances after test
	defer func() {
		keystoneInstance = originalKeystone
		globalKeystoneInstance = originalGlobalKeystone
	}()

	// Set policy file path - this is crucial
	viper.Set("keystone.policy_file", "../test/policy.json")
	viper.Set("maia.label_value_ttl", "72h")

	// Setup storage mock
	storageMock := storage.NewMockDriver(ctrl)

	// Reset prometheus registry to avoid conflicts
	prometheus.DefaultRegisterer = prometheus.NewPedanticRegistry()

	// Setup router with both keystones
	router := setupRouter(regularKeystone, globalKeystone, storageMock)

	// Test case: redirect with global param preserves the param
	t.Run("Redirect preserves global param", func(t *testing.T) {
		// Create request with global parameter
		req := httptest.NewRequest(http.MethodGet, "/graph?global=true", http.NoBody)

		// Execute request
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		// Check status code
		resp := recorder.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode, "Expected redirect")

		// Check Location header contains global parameter
		location := resp.Header.Get("Location")
		assert.Contains(t, location, "global=true", "Redirect should preserve global flag")
	})

	// Test case: redirect with global header adds global param
	t.Run("Redirect with global header", func(t *testing.T) {
		// Create request with global header
		req := httptest.NewRequest(http.MethodGet, "/graph", http.NoBody)
		req.Header.Set("X-Global-Region", "true")

		// Execute request
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		// Check status code
		resp := recorder.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode, "Expected redirect")

		// Check Location header contains global parameter
		location := resp.Header.Get("Location")
		assert.Contains(t, location, "global=true", "Redirect should add global flag from header")
	})
}

func TestTokenLogin_success(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	// The POST /auth endpoint uses guessScope=true like the graph endpoint.
	// Include X-Auth-Token in injected headers so setAuthCookies() can set the cookie.
	headerWithToken := map[string]string{
		"X-User-Id":          projectContext.Auth["user_id"],
		"X-User-Name":        projectContext.Auth["user_name"],
		"X-User-Domain-Name": projectContext.Auth["user_domain_name"],
		"X-Project-Id":       projectContext.Auth["project_id"],
		"X-Project-Name":     projectContext.Auth["project_name"],
		"X-Auth-Token":       "someverylongtokenideed",
	}
	httpReqMatcher := test.HTTPRequestMatcher{InjectHeader: headerWithToken}
	keystoneMock.EXPECT().AuthenticateRequest(test.MatchContext(), httpReqMatcher, true).Return(projectContext, nil)

	// POST form body with x-auth-token (the secure alternative to URL query param)
	req := httptest.NewRequest(http.MethodPost, "/testdomain/auth", strings.NewReader("x-auth-token=someverylongtokenideed"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	resp := recorder.Result()
	defer resp.Body.Close()

	// Expect 303 See Other redirect to the graph page
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode, "Expected redirect to graph")

	// Check redirect target
	location := resp.Header.Get("Location")
	assert.Equal(t, "/testdomain/graph", location, "Should redirect to domain graph page")

	// Check that auth cookie was set
	cookies := resp.Cookies()
	var tokenCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "X-Auth-Token" {
			tokenCookie = c
			break
		}
	}
	assert.NotNil(t, tokenCookie, "Auth cookie should be set")
	if tokenCookie != nil {
		assert.True(t, tokenCookie.HttpOnly, "Cookie should be HttpOnly")
		assert.True(t, tokenCookie.Secure, "Cookie should be Secure")
	}
}

func TestTokenLogin_failAuth(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	httpReqMatcher := test.HTTPRequestMatcher{InjectHeader: projectHeader}
	keystoneMock.EXPECT().AuthenticateRequest(test.MatchContext(), httpReqMatcher, true).Return(nil, keystone.NewAuthenticationError(keystone.StatusWrongCredentials, "invalid token"))

	req := httptest.NewRequest(http.MethodPost, "/testdomain/auth", strings.NewReader("x-auth-token=invalidtoken"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	resp := recorder.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Invalid token should return 401")
}

func TestTokenLogin_noToken(t *testing.T) {
	ctrl := gomock.NewController(t)

	router, keystoneMock, _ := setupTest(t, ctrl)

	// With no token in POST body and no other credentials, auth should fail
	httpReqMatcher := test.HTTPRequestMatcher{InjectHeader: projectHeader}
	keystoneMock.EXPECT().AuthenticateRequest(test.MatchContext(), httpReqMatcher, true).Return(nil, keystone.NewAuthenticationError(keystone.StatusMissingCredentials, "Authorization header missing"))

	req := httptest.NewRequest(http.MethodPost, "/testdomain/auth", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	resp := recorder.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Missing token should return 401")
}

func TestTokenLogin_bodyTooLarge(t *testing.T) {
	prometheus.DefaultRegisterer = prometheus.NewPedanticRegistry()
	keystoneInstance = nil
	globalKeystoneInstance = nil

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockKeystone := keystone.NewMockDriver(ctrl)
	mockStorage := storage.NewMockDriver(ctrl)

	// With a too-large body, ParseForm fails, no token extracted,
	// so AuthenticateRequest is called with no credentials and should return error.
	mockKeystone.EXPECT().ServiceURL().Return("http://localhost:9091").AnyTimes()
	mockKeystone.EXPECT().AuthenticateRequest(gomock.Any(), gomock.Any(), true).Return(
		nil, keystone.NewAuthenticationError(keystone.StatusMissingCredentials, "missing credentials"))

	router := setupRouter(mockKeystone, nil, mockStorage)

	// Create body larger than 16KB
	largeBody := strings.Repeat("x-auth-token=", 2000) // ~26KB
	req := httptest.NewRequest(http.MethodPost, "/testdomain/auth", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should get 401 (missing credentials) since token extraction failed
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTokenLogin_wrongContentType(t *testing.T) {
	prometheus.DefaultRegisterer = prometheus.NewPedanticRegistry()
	keystoneInstance = nil
	globalKeystoneInstance = nil

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockKeystone := keystone.NewMockDriver(ctrl)
	mockStorage := storage.NewMockDriver(ctrl)

	// With wrong Content-Type, body is not parsed, no token found → missing credentials
	mockKeystone.EXPECT().ServiceURL().Return("http://localhost:9091").AnyTimes()
	mockKeystone.EXPECT().AuthenticateRequest(gomock.Any(), gomock.Any(), true).Return(
		nil, keystone.NewAuthenticationError(keystone.StatusMissingCredentials, "missing credentials"))

	router := setupRouter(mockKeystone, nil, mockStorage)

	body := `{"x-auth-token": "someverylongtokenideed"}`
	req := httptest.NewRequest(http.MethodPost, "/testdomain/auth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should get 401 — JSON body not parsed as form data
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
