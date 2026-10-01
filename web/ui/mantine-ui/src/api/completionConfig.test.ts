// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

import { HTTPPrometheusClient } from "@prometheus-io/codemirror-promql/dist/esm/client/prometheus";
import { completionConfig } from "./completionConfig";
import { vi } from "vitest";

// Exercise the installed client: its default apiPrefix and method caused the
// regression, which tests of the string helper alone cannot detect.
describe.each(["/ui", "/domain/project/metrics/maia/ui"])("completion at %s", (prefix) => {
  afterEach(() => vi.unstubAllGlobals());

  test("labels, series, values and metadata use GET and the selected project", async () => {
    const fetch = vi.fn().mockImplementation(async () =>
      new Response(JSON.stringify({ status: "success", data: [] }))
    );
    vi.stubGlobal("fetch", fetch);
    const client = new HTTPPrometheusClient(completionConfig(prefix, "selected-project"));
    await client.labelNames("up");
    await client.series("up");
    await client.labelValues("job");
    await client.metricMetadata();

    expect(fetch).toHaveBeenCalledTimes(4);
    const root = prefix.slice(0, -3);
    const paths = fetch.mock.calls.map(([input, init]) => {
      const url = new URL(input, window.location.origin);
      expect(url.searchParams.get("project_id")).toBe("selected-project");
      expect(init.method ?? "GET").toBe("GET");
      expect(init.body ?? null).toBeNull();
      expect(init.credentials).toBe("same-origin");
      return url.pathname;
    });
    expect(paths).toEqual([
      `${root}/api/v1/labels`, `${root}/api/v1/series`,
      `${root}/api/v1/label/job/values`, `${root}/api/v1/metadata`,
    ]);
    expect(new URL(fetch.mock.calls[0][0], window.location.origin).searchParams.get("match[]")).toContain("up");
  });
});
