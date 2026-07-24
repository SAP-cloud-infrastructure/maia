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
      maiaFetch("/api/v1/whoami").then((r) => r.json()),
      maiaFetch("/api/v1/projects").then((r) => r.json()),
    ]).then(([whoami, projectList]: [MaiaUser, MaiaProject[]]) => {
      setUser(whoami);
      setProjects(projectList);

      const savedId = localStorage.getItem(LOCAL_STORAGE_KEY);
      const saved = projectList.find((p) => p.id === savedId);
      setCurrentProject(saved ?? projectList[0] ?? null);
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
