// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

// MAIA: keep completion requests on the same API and project as query panels.
import type { PrometheusConfig } from "@prometheus-io/codemirror-promql/dist/esm/client";
import { apiBase } from "./apiBase";
import { maiaFetch } from "../lib/maiaFetch";

export function completionConfig(
  pathPrefix: string,
  projectId: string | undefined
): PrometheusConfig {
  return {
    url: "",
    // An empty apiPrefix does not override the client's default /api/v1.
    apiPrefix: apiBase(pathPrefix),
    // Maia's read API uses GET, including labels and series.
    httpMethod: "GET",
    fetchFn: (input, init) => {
      const url = new URL(String(input), window.location.origin);
      if (projectId) url.searchParams.set("project_id", projectId);
      return maiaFetch(`${url.pathname}${url.search}`, init);
    },
  };
}
