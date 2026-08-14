// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/SAP-cloud-infrastructure/maia/pkg/keystone"
	"github.com/SAP-cloud-infrastructure/maia/pkg/test"
)

// The client-controlled project_id query-param fallback must verify the
// authenticated user actually has a monitoring role on the requested project
// before trusting it. Without this check, any authenticated user could read
// another tenant's metrics by supplying a foreign project_id (IDOR).

// scopeReq builds a request carrying the given X-User-Id header and project_id
// query param, with no scope headers set, so scopeToLabelConstraint falls
// through to the query-param branch.
func scopeReq(userID, projectID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "http://maia.local/api/v1/query?query=up&project_id="+projectID, http.NoBody)
	req.Header.Set("X-User-Id", userID)
	return req
}

func TestScopeQueryParam_authorizedMember(t *testing.T) {
	ctrl := gomock.NewController(t)
	sentinelValue = ""
	ks := keystone.NewMockDriver(ctrl)

	// user u1 is a member of project p1
	ks.EXPECT().UserProjects(test.MatchContext(), "u1").
		Return([]tokens.Scope{{ProjectID: "p1"}}, nil)
	ks.EXPECT().ChildProjects(test.MatchContext(), "p1").
		Return([]string{"p1child"}, nil)

	labelKey, labelValues := scopeToLabelConstraint(scopeReq("u1", "p1"), ks)

	assert.Equal(t, "project_id", labelKey)
	assert.EqualValues(t, []string{"p1", "p1child"}, labelValues,
		"authorized member should get their project plus its children")
}

func TestScopeQueryParam_foreignProjectRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	sentinelValue = ""
	ks := keystone.NewMockDriver(ctrl)

	// user u1 is a member of p1 only, but requests victim project p-victim
	ks.EXPECT().UserProjects(test.MatchContext(), "u1").
		Return([]tokens.Scope{{ProjectID: "p1"}}, nil)
	// ChildProjects must never be called for a project the user cannot access.

	assert.Panics(t, func() {
		scopeToLabelConstraint(scopeReq("u1", "p-victim"), ks)
	}, "requesting a project the user is not a member of must be rejected (panic), not resolved")
}

// TestScopeHeaderWinsUnchecked documents the load-bearing cross-file invariant:
// the X-Project-Id header branch is trusted WITHOUT any membership check and
// takes precedence over the (checked) query-param branch. That is only safe
// because keystone.AuthenticateRequest strips inbound client copies of these
// scope headers and re-sets them from the validated token — proven by
// TestAuthenticateRequest in pkg/keystone (it asserts a forged X-Project-Id is
// overwritten and X-Domain-Id cleared). This test is the pkg/api half of that
// pair: it demonstrates that IF a client-controlled scope header ever reached
// here, it would resolve unchecked — so the keystone-side strip must never be
// removed. UserProjects/membership are intentionally NOT expected: the header
// branch does not consult them.
func TestScopeHeaderWinsUnchecked(t *testing.T) {
	ctrl := gomock.NewController(t)
	sentinelValue = ""
	ks := keystone.NewMockDriver(ctrl)

	// Header branch expands children but performs no membership check.
	ks.EXPECT().ChildProjects(test.MatchContext(), "p-server-set").
		Return([]string{}, nil)

	// A request whose X-Project-Id was set by AuthenticateRequest (server-side),
	// with a stale query param that must be ignored due to header precedence.
	req := httptest.NewRequest(http.MethodGet, "http://maia.local/api/v1/query?query=up&project_id=p-ignored", http.NoBody)
	req.Header.Set("X-Project-Id", "p-server-set")

	labelKey, labelValues := scopeToLabelConstraint(req, ks)

	assert.Equal(t, "project_id", labelKey)
	assert.EqualValues(t, []string{"p-server-set"}, labelValues,
		"header branch must resolve exactly the server-set X-Project-Id (query param ignored, no membership check)")
}
