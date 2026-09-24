// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// MAIA: single source of truth for the API base path.
// This file is Maia-specific — do not overwrite during upstream syncs.
//
// Why this exists:
// Maia's API lives at the server root under /api/v1, while the SPA is served
// under /ui/ (BrowserRouter basename). When Maia is served directly at its own
// origin, the API base is simply "/api/v1". When Maia is served through a
// reverse proxy under a sub-path (e.g. Elektra proxies it at
// /<domain>/metrics/maia/), the API is reachable at
// <proxyPrefix>/api/v1 — NOT at the origin root — so a hardcoded "/api/v1"
// would miss the proxy prefix and 404.
//
// The SPA already derives its serving prefix at runtime via getPathPrefix()
// (state/settingsSlice.ts), which drives <BrowserRouter basename={pathPrefix}>.
// That prefix includes the trailing "/ui" segment for the SPA routes. The API
// sits one level up from /ui (at the proxy root's /api/v1), so we strip a
// trailing "/ui" from the served prefix and append "/api/v1".
//
//   Standalone   served at /ui/query        → prefix "/ui"
//                → strip "/ui" → ""          → base "/api/v1"        (unchanged)
//   Proxied      served at
//                /d/metrics/maia/ui/query    → prefix "/d/metrics/maia/ui"
//                → strip "/ui" → "/d/metrics/maia"
//                → base "/d/metrics/maia/api/v1"
//
// apiBase(prefix) is pure so it can be unit-tested without a DOM. Callers pass
// the runtime pathPrefix from useSettings().

// API_PATH is the API root path segment, always appended after the (possibly
// empty) proxy prefix. Kept exported for callers that only need the suffix.
export const API_PATH = "/api/v1";

// apiBase returns the fully-resolved API base for a given served pathPrefix.
// pathPrefix is the value getPathPrefix() computed from window.location
// (e.g. "/ui", "", or "/d/metrics/maia/ui").
export function apiBase(pathPrefix: string): string {
  return `${servedRoot(pathPrefix)}${API_PATH}`;
}

// servedRoot returns the deployment root — the path the whole Maia deployment
// is served under, with the SPA's trailing "/ui" segment removed. This is the
// correct base for server-root endpoints that live OUTSIDE /api/v1, such as
// /-/ready. Standalone → "" (so "/-/ready"); proxied under /d/metrics/maia/ui
// → "/d/metrics/maia" (so "/d/metrics/maia/-/ready").
export function servedRoot(pathPrefix: string): string {
  // Strip a single trailing "/ui" (the SPA segment) so paths resolve at the
  // proxy root, not under the SPA path. Anything else is preserved verbatim.
  return pathPrefix.endsWith("/ui")
    ? pathPrefix.slice(0, -"/ui".length)
    : pathPrefix;
}
