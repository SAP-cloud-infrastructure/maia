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
  setProject: (project: MaiaProject) => void;
}

const LOCAL_STORAGE_KEY = "maia_project_id";

const MaiaProjectContext = createContext<MaiaProjectContextValue>({
  user: null,
  projects: [],
  currentProject: null,
  setProject: () => undefined,
});

export const MaiaProjectProvider: FC<PropsWithChildren> = ({ children }) => {
  const [user, setUser] = useState<MaiaUser | null>(null);
  const [projects, setProjects] = useState<MaiaProject[]>([]);
  const [currentProject, setCurrentProject] = useState<MaiaProject | null>(null);

  useEffect(() => {
    Promise.all([
      // MAIA: absolute paths — app is served at /ui/, relative URLs would resolve to /ui/api/v1/...
      maiaFetch("/api/v1/whoami"),
      maiaFetch("/api/v1/projects"),
    ]).then(async ([whoamiRes, projectsRes]) => {
      if (!whoamiRes.ok || !projectsRes.ok) {
        // Token missing or invalid — leave user/projects null, UI stays empty
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
      const saved = projectList.find((p) => p.id === savedId);
      setCurrentProject(saved ?? projectList[0] ?? null);
    }).catch(() => {
      // Network error — fail silently, project switcher stays hidden
    });
  }, []);

  const setProject = useCallback((project: MaiaProject) => {
    localStorage.setItem(LOCAL_STORAGE_KEY, project.id);
    setCurrentProject(project);
  }, []);

  return (
    <MaiaProjectContext.Provider value={{ user, projects, currentProject, setProject }}>
      {children}
    </MaiaProjectContext.Provider>
  );
};

export const useMaiaProject = () => useContext(MaiaProjectContext);
