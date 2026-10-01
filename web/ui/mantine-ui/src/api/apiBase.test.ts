// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

import { apiBase, servedRoot, API_PATH } from "./apiBase";

// MAIA: these tests lock in the contract that makes Maia's SPA servable both
// at its own origin root AND behind a reverse proxy under a sub-path. The whole
// SSO-through-a-proxy approach depends on this resolution being correct, so the
// standalone case must stay byte-identical to the historical hardcoded "/api/v1".

describe("apiBase", () => {
  test("standalone: /ui prefix strips to /api/v1 (no regression)", () => {
    // getPathPrefix() yields "/ui" when served at Maia's own root (/ui/query).
    expect(apiBase("/ui")).toBe("/api/v1");
  });

  test("standalone: empty prefix also yields /api/v1", () => {
    // Defensive: if the prefix ever resolves to "" the API base is still /api/v1.
    expect(apiBase("")).toBe("/api/v1");
  });

  test("proxied: sub-path prefix resolves under the proxy root", () => {
    // Served at /d/metrics/maia/ui/query → prefix "/d/metrics/maia/ui".
    expect(apiBase("/d/metrics/maia/ui")).toBe("/d/metrics/maia/api/v1");
  });

  test("proxied: single-segment sub-path", () => {
    expect(apiBase("/metrics/ui")).toBe("/metrics/api/v1");
  });

  test("only a trailing /ui is stripped, not an interior one", () => {
    // A literal "/ui" appearing earlier must be preserved.
    expect(apiBase("/ui/team/ui")).toBe("/ui/team/api/v1");
  });

  test("a prefix that merely ends in 'ui' (no slash) is not stripped", () => {
    // "/gui" ends with "ui" but not "/ui"; must be preserved verbatim.
    expect(apiBase("/gui")).toBe("/gui/api/v1");
  });
});

describe("servedRoot", () => {
  test("standalone: /ui prefix strips to empty root", () => {
    expect(servedRoot("/ui")).toBe("");
  });

  test("empty prefix stays empty", () => {
    expect(servedRoot("")).toBe("");
  });

  test("proxied: sub-path strips the trailing /ui", () => {
    expect(servedRoot("/d/metrics/maia/ui")).toBe("/d/metrics/maia");
  });

  test("root-relative endpoints resolve correctly in both deployments", () => {
    // /-/ready is a server-root endpoint (not under /api/v1).
    expect(`${servedRoot("/ui")}/-/ready`).toBe("/-/ready");
    expect(`${servedRoot("/d/metrics/maia/ui")}/-/ready`).toBe(
      "/d/metrics/maia/-/ready"
    );
  });
});

describe("API_PATH", () => {
  test("is the historical API root segment", () => {
    expect(API_PATH).toBe("/api/v1");
  });

  test("apiBase always ends with API_PATH", () => {
    for (const prefix of ["/ui", "", "/d/metrics/maia/ui", "/gui"]) {
      expect(apiBase(prefix).endsWith(API_PATH)).toBe(true);
    }
  });
});
