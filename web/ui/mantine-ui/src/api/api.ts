import { QueryKey, useQuery, useSuspenseQuery } from "@tanstack/react-query";
// MAIA: replaced fetch with maiaFetch for X-Auth-Token injection
import { maiaFetch } from "../lib/maiaFetch";
// MAIA: inject project_id into every API call
import { useMaiaProject } from "../context/MaiaProjectContext";
// MAIA: resolve the API base from the served path (supports reverse-proxy sub-paths)
import { apiBase, API_PATH } from "./apiBase";
import { useSettings } from "../state/settingsSlice";

// Re-exported for callers that imported API_PATH from here historically.
export { API_PATH };

export type SuccessAPIResponse<T> = {
  status: "success";
  data: T;
  warnings?: string[];
  infos?: string[];
};

export type ErrorAPIResponse = {
  status: "error";
  errorType: string;
  error: string;
};

export type APIResponse<T> = SuccessAPIResponse<T> | ErrorAPIResponse;

const createQueryFn =
  <T>({
    apiBasePath,
    path,
    params,
    recordResponseTime,
  }: {
    apiBasePath: string;
    path: string;
    params?: Record<string, string>;
    recordResponseTime?: (time: number) => void;
  }) =>
  async ({ signal }: { signal: AbortSignal }) => {
    const queryString = params
      ? `?${new URLSearchParams(params).toString()}`
      : "";

    try {
      const startTime = Date.now();

      // MAIA: apiBasePath is derived from the served path (apiBase()), so the
      // same bundle works served at Maia's own root (/api/v1) or behind a
      // reverse proxy under a sub-path (<prefix>/api/v1).
      const res = await maiaFetch(
        `${apiBasePath}${path}${queryString}`,
        {
          cache: "no-store",
          signal,
        }
      );

      if (
        !res.ok &&
        !res.headers.get("content-type")?.startsWith("application/json")
      ) {
        // For example, Prometheus may send a 503 Service Unavailable response
        // with a "text/plain" content type when it's starting up. But the API
        // may also respond with a JSON error message and the same error code.
        throw new Error(res.statusText);
      }

      const apiRes = (await res.json()) as APIResponse<T>;

      if (recordResponseTime) {
        recordResponseTime(Date.now() - startTime);
      }

      if (apiRes.status === "error") {
        throw new Error(
          apiRes.error !== undefined
            ? apiRes.error
            : 'missing "error" field in response JSON'
        );
      }

      return apiRes as SuccessAPIResponse<T>;
    } catch (error) {
      if (!(error instanceof Error)) {
        throw new Error("Unknown error");
      }

      switch (error.name) {
        case "TypeError":
          throw new Error("Network error or unable to reach the server");
        case "SyntaxError":
          throw new Error("Invalid JSON response");
        default:
          throw error;
      }
    }
  };

type QueryOptions = {
  key?: QueryKey;
  path: string;
  params?: Record<string, string>;
  enabled?: boolean;
  refetchInterval?: false | number;
  recordResponseTime?: (time: number) => void;
};

export const useAPIQuery = <T>({
  key,
  path,
  params,
  enabled,
  recordResponseTime,
  refetchInterval,
}: QueryOptions) => {
  // MAIA: inject project_id into every API call
  const { currentProject } = useMaiaProject();
  // MAIA: resolve API base from the served path
  const { pathPrefix } = useSettings();
  const maiaParams = currentProject
    ? { ...params, project_id: currentProject.id }
    : params;

  return useQuery<SuccessAPIResponse<T>>({
    queryKey: key !== undefined ? key : [path, maiaParams],
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: refetchInterval,
    gcTime: 0,
    // MAIA: don't fire until project is loaded — avoids scope-less queries
    // that would silently fall back to the Keystone token scope instead of
    // the project the user selected in the switcher.
    enabled: enabled !== false && currentProject !== null,
    queryFn: createQueryFn({
      apiBasePath: apiBase(pathPrefix),
      path,
      params: maiaParams,
      recordResponseTime,
    }),
  });
};

export const useSuspenseAPIQuery = <T>({ key, path, params }: QueryOptions) => {
  // MAIA: inject project_id into every API call
  const { currentProject } = useMaiaProject();
  // MAIA: resolve API base from the served path
  const { pathPrefix } = useSettings();
  const maiaParams = currentProject
    ? { ...params, project_id: currentProject.id }
    : params;

  return useSuspenseQuery<SuccessAPIResponse<T>>({
    queryKey: key !== undefined ? key : [path, maiaParams],
    retry: false,
    refetchOnWindowFocus: false,
    staleTime: Infinity,
    gcTime: 0,
    queryFn: createQueryFn({ apiBasePath: apiBase(pathPrefix), path, params: maiaParams }),
  });
};
