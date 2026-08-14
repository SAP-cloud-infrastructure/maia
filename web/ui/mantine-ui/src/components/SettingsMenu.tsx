import {
  Popover,
  ActionIcon,
  Fieldset,
  Checkbox,
  Stack,
} from "@mantine/core";
import { IconSettings } from "@tabler/icons-react";
import { FC } from "react";
import { useAppDispatch } from "../state/hooks";
import { updateSettings, useSettings } from "../state/settingsSlice";
import { actionIconStyle } from "../styles";

const SettingsMenu: FC = () => {
  const {
    useLocalTime,
    enableQueryHistory,
    enableAutocomplete,
    enableSyntaxHighlighting,
    enableLinter,
    showQueryWarnings,
    showQueryInfoNotices,
  } = useSettings();
  const dispatch = useAppDispatch();

  return (
    <Popover position="bottom" withArrow shadow="md">
      <Popover.Target>
        <ActionIcon
          color="gray"
          title="Settings"
          aria-label="Settings"
          size={32}
        >
          <IconSettings style={actionIconStyle} />
        </ActionIcon>
      </Popover.Target>
      <Popover.Dropdown>
        <Stack>
          <Fieldset p="md" legend="Global settings">
            <Checkbox
              checked={useLocalTime}
              label="Use local time"
              onChange={(event) =>
                dispatch(updateSettings({ useLocalTime: event.currentTarget.checked }))
              }
            />
          </Fieldset>

          <Fieldset p="md" legend="Query page settings">
            <Stack>
              <Checkbox
                checked={enableQueryHistory}
                label="Enable query history"
                onChange={(event) =>
                  dispatch(updateSettings({ enableQueryHistory: event.currentTarget.checked }))
                }
              />
              <Checkbox
                checked={enableAutocomplete}
                label="Enable autocomplete"
                onChange={(event) =>
                  dispatch(updateSettings({ enableAutocomplete: event.currentTarget.checked }))
                }
              />
              <Checkbox
                checked={enableSyntaxHighlighting}
                label="Enable syntax highlighting"
                onChange={(event) =>
                  dispatch(updateSettings({ enableSyntaxHighlighting: event.currentTarget.checked }))
                }
              />
              <Checkbox
                checked={enableLinter}
                label="Enable linter"
                onChange={(event) =>
                  dispatch(updateSettings({ enableLinter: event.currentTarget.checked }))
                }
              />
              <Checkbox
                checked={showQueryWarnings}
                label="Show query warnings"
                onChange={(event) =>
                  dispatch(updateSettings({ showQueryWarnings: event.currentTarget.checked }))
                }
              />
              <Checkbox
                checked={showQueryInfoNotices}
                label="Show query info notices"
                onChange={(event) =>
                  dispatch(updateSettings({ showQueryInfoNotices: event.currentTarget.checked }))
                }
              />
            </Stack>
          </Fieldset>
          {/* MAIA: removed Alerts page settings and Rules page settings — not applicable */}
        </Stack>
      </Popover.Dropdown>
    </Popover>
  );
};

export default SettingsMenu;
