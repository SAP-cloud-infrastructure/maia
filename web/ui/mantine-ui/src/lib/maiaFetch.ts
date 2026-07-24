// MAIA: wraps fetch() to inject X-Auth-Token from cookie on every request.
// This file is Maia-specific — do not overwrite during upstream syncs.
export function maiaFetch(url: string, init?: RequestInit): Promise<Response> {
  const token = document.cookie
    .split("; ")
    .find((r) => r.startsWith("X-Auth-Token="))
    ?.split("=")[1];
  return fetch(url, {
    ...init,
    headers: {
      ...init?.headers,
      ...(token ? { "X-Auth-Token": token } : {}),
    },
    credentials: "same-origin",
  });
}
