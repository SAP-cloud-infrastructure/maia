// MAIA: wraps fetch() ensuring credentials are included on every request.
// This file is Maia-specific — do not overwrite during upstream syncs.
//
// Auth note: The server sets X-Auth-Token as an HttpOnly cookie, which means
// it is invisible to JavaScript (document.cookie cannot read it). We rely on
// credentials: "same-origin" so the browser automatically includes all cookies
// (including HttpOnly ones) on same-origin requests. The server's
// authenticateOnly() and authorizeRules() then read the cookie into the
// X-Auth-Token header internally (pkg/api/util.go).
export function maiaFetch(url: string, init?: RequestInit): Promise<Response> {
  return fetch(url, {
    ...init,
    credentials: "same-origin",
  });
}
