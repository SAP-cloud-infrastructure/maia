// MAIA: project switcher navbar component — shows current project dropdown
// and logged-in username. Maia-specific — do not overwrite during upstream syncs.
import { Anchor, Group, Select, Text } from "@mantine/core";
import { useMaiaProject } from "../context/MaiaProjectContext";

export function MaiaProjectSwitcher() {
  const { user, projects, currentProject, setProject } = useMaiaProject();

  if (!currentProject) return null;

  const options = projects.map((p) => ({ value: p.id, label: p.name }));

  return (
    <Group gap="xs" wrap="nowrap">
      {user && (
        <Text size="sm" c="gray.3" style={{ whiteSpace: "nowrap" }}>
          {user.userName}
        </Text>
      )}
      <Select
        size="xs"
        data={options}
        value={currentProject.id}
        onChange={(id) => {
          const p = projects.find((p) => p.id === id);
          if (p) setProject(p);
        }}
        styles={{ input: { minWidth: 140 } }}
        allowDeselect={false}
      />
      {/* MAIA: link back to classic UI */}
      <Anchor
        href={`/${user?.userDomainName ?? "Default"}/graph?project_id=${currentProject.id}`}
        size="xs"
        c="dimmed"
        style={{ whiteSpace: "nowrap" }}
      >
        Classic UI
      </Anchor>
    </Group>
  );
}
