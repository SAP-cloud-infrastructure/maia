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
function getCookie(name: string): string | undefined {
  return document.cookie
    .split("; ")
    .find((r) => r.startsWith(`${name}=`))
    ?.split("=")[1];
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

  useEffect(() => {
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
          // X-User-Domain-Name is a non-HttpOnly cookie (readable by JS) set
          // by the server after a successful login to remember the user's domain.
          const domain = getCookie("X-User-Domain-Name") ?? "Default";
          window.location.href = `/${domain}/graph`;
          return;
        }
        // Other error (5xx, network issue) — fail gracefully, stay on page
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
    }).catch(() => {
      // Network error — fail gracefully, stay on page
      setIsLoading(false);
    });
  }, []);

  const setProject = useCallback((project: MaiaProject) => {
    localStorage.setItem(LOCAL_STORAGE_KEY, project.id);
    setCurrentProject(project);
  }, []);

  return (
    <MaiaProjectContext.Provider value={{ user, projects, currentProject, isLoading, setProject }}>
      {children}
    </MaiaProjectContext.Provider>
  );
};

export const useMaiaProject = () => useContext(MaiaProjectContext);
