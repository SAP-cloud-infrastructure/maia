// MAIA: project context — fetches whoami + projects on mount, exposes
// currentProject / setProject, persists selection to localStorage.
// This file is Maia-specific — do not overwrite during upstream syncs.
import {
  createContext,
  FC,
  PropsWithChildren,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import { Alert, Button, Center, Loader, Stack } from "@mantine/core";
import { maiaFetch } from "../lib/maiaFetch";

export interface MaiaUser {
  userId: string;
  userName: string;
  userDomainName: string;
  projectId: string;
  projectName: string;
  domainId: string;
  domainName: string;
  roles: string[];
}

export interface MaiaProject {
  id: string;
  name: string;
}

interface MaiaProjectContextValue {
  user: MaiaUser | null;
  projects: MaiaProject[];
  currentProject: MaiaProject | null;
  // isLoading is true during the initial whoami/projects fetch.
  // Consumers can use this to show a skeleton instead of empty state.
  isLoading: boolean;
  setProject: (project: MaiaProject) => void;
}

const LOCAL_STORAGE_KEY = "maia_project_id";

// Read a non-HttpOnly cookie value by name (HttpOnly cookies are JS-invisible).
// Splits on the first "=" only (cookie values may legitimately contain "=") and
// defensively URL-decodes the value. The server sets the raw domain name and
// relies on Go's http.Cookie value sanitization rather than explicit
// percent-encoding, so decoding is a no-op for well-formed names but guards
// against any encoded characters round-tripping.
function getCookie(name: string): string | undefined {
  const entry = document.cookie
    .split("; ")
    .find((r) => r.startsWith(`${name}=`));
  if (entry === undefined) return undefined;
  const value = entry.slice(name.length + 1);
  try {
    return decodeURIComponent(value);
  } catch {
    // Malformed percent-encoding — fall back to the raw value rather than throw.
    return value;
  }
}

const MaiaProjectContext = createContext<MaiaProjectContextValue>({
  user: null,
  projects: [],
  currentProject: null,
  isLoading: true,
  setProject: () => undefined,
});

export const MaiaProjectProvider: FC<PropsWithChildren> = ({ children }) => {
  const [user, setUser] = useState<MaiaUser | null>(null);
  const [projects, setProjects] = useState<MaiaProject[]>([]);
  const [currentProject, setCurrentProject] = useState<MaiaProject | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  // MAIA: non-401 load failures surface here instead of silently rendering an
  // empty app. reloadKey bumps to re-run the fetch effect on retry.
  const [loadError, setLoadError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    setIsLoading(true);
    setLoadError(null);
    Promise.all([
      // MAIA: absolute paths — app is served at /ui/, relative URLs would resolve to /ui/api/v1/...
      maiaFetch("/api/v1/whoami"),
      maiaFetch("/api/v1/projects"),
    ]).then(async ([whoamiRes, projectsRes]) => {
      if (!whoamiRes.ok || !projectsRes.ok) {
        const status = !whoamiRes.ok ? whoamiRes.status : projectsRes.status;
        if (status === 401) {
          // No valid session — redirect to the classic UI for login.
          // The classic UI shows a Basic Auth prompt and sets the auth cookie.
          // X-User-Domain-Name is set by the server as a JS-readable (non-HttpOnly)
          // cookie after login so this redirect can target the user's real domain;
          // falls back to "Default" when absent OR empty (|| not ??) — an empty
          // domain would produce a protocol-relative "//graph" redirect.
          const domain = getCookie("X-User-Domain-Name") || "Default";
          window.location.href = `/${encodeURIComponent(domain)}/graph`;
          return;
        }
        // Other error (5xx, network issue) — surface it instead of rendering
        // an empty app; the retry button re-runs this effect.
        console.error(
          `MaiaProjectContext: whoami/projects fetch failed with status ${status}`
        );
        setLoadError(`Could not load your projects (HTTP ${status}).`);
        setIsLoading(false);
        return;
      }

      const whoami: MaiaUser = await whoamiRes.json();
      const projectList: MaiaProject[] = await projectsRes.json();

      setUser(whoami);

      // MAIA: deduplicate by project ID — QA Keystone may return the same project
      // multiple times when a user has more than one role assignment on it.
      const seen = new Set<string>();
      const unique = projectList.filter((p) => {
        if (seen.has(p.id)) return false;
        seen.add(p.id);
        return true;
      });
      setProjects(unique);

      const savedId = localStorage.getItem(LOCAL_STORAGE_KEY);
      const saved = unique.find((p) => p.id === savedId);
      setCurrentProject(saved ?? unique[0] ?? null);
      setIsLoading(false);
    }).catch((err) => {
      // Network error / thrown exception in the success path — surface it.
      console.error("MaiaProjectContext: whoami/projects fetch failed", err);
      setLoadError("Could not reach the server. Check your connection.");
      setIsLoading(false);
    });
  }, [reloadKey]);

  const setProject = useCallback((project: MaiaProject) => {
    localStorage.setItem(LOCAL_STORAGE_KEY, project.id);
    setCurrentProject(project);
  }, []);

  // MAIA: surface a non-401 load failure with a retry instead of silently
  // rendering a project-less (empty) app.
  if (loadError) {
    return (
      <Center h="100vh">
        <Stack align="center">
          <Alert color="red" title="Could not load your projects" maw={480}>
            {loadError}
          </Alert>
          <Button onClick={() => setReloadKey((v) => v + 1)}>Retry</Button>
        </Stack>
      </Center>
    );
  }

  // MAIA: don't render children until the initial whoami/projects fetch
  // resolves. This gates every downstream query — including useSuspenseQuery
  // callers, which cannot be disabled per-query (skipToken is not assignable
  // to a suspense query) — so no request fires before a project scope exists.
  // Without this, scope-less queries would silently fall back to the Keystone
  // token scope instead of the project selected in the switcher. On a 401 the
  // effect redirects away, so rendering nothing here is correct.
  if (isLoading) {
    return (
      <Center h="100vh">
        <Loader />
      </Center>
    );
  }

  return (
    <MaiaProjectContext.Provider value={{ user, projects, currentProject, isLoading, setProject }}>
      {children}
    </MaiaProjectContext.Provider>
  );
};

export const useMaiaProject = () => useContext(MaiaProjectContext);
