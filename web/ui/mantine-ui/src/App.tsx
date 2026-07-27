import "@mantine/core/styles.css";
import "@mantine/code-highlight/styles.css";
import "@mantine/notifications/styles.css";
import "@mantine/dates/styles.css";
import "./mantine-overrides.css";
import classes from "./App.module.css";

import {
  AppShell,
  Box,
  Burger,
  Button,
  Group,
  MantineProvider,
  Skeleton,
  Text,
  createTheme,
} from "@mantine/core";
import { useDisclosure } from "@mantine/hooks";
import {
  IconDeviceDesktopAnalytics,
  IconSearch,
} from "@tabler/icons-react";
import {
  BrowserRouter,
  Link,
  NavLink,
  Navigate,
  Route,
  Routes,
} from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import QueryPage from "./pages/query/QueryPage";
import AgentPage from "./pages/AgentPage";
import { Suspense } from "react";
import ErrorBoundary from "./components/ErrorBoundary";
import { ThemeSelector } from "./components/ThemeSelector";
import { Notifications } from "@mantine/notifications";
import { useSettings } from "./state/settingsSlice";
import SettingsMenu from "./components/SettingsMenu";
import ReadinessWrapper from "./components/ReadinessWrapper";
// MAIA: disabled — NotificationsProvider and NotificationsIcon use /api/v1/notifications/live SSE
// import NotificationsProvider from "./components/NotificationsProvider";
// import NotificationsIcon from "./components/NotificationsIcon";
import { QueryParamProvider } from "use-query-params";
import { ReactRouter6Adapter } from "use-query-params/adapters/react-router-6";
import { navIconStyle } from "./styles";
// MAIA: project context provider and switcher
import { MaiaProjectProvider } from "./context/MaiaProjectContext";
import { MaiaProjectSwitcher } from "./components/MaiaProjectSwitcher";

import {
  CodeHighlightAdapterProvider,
  createHighlightJsAdapter,
} from "@mantine/code-highlight";
import hljs from "highlight.js/lib/core";
import "./highlightjs.css";
import yamlLang from "highlight.js/lib/languages/yaml";

hljs.registerLanguage("yaml", yamlLang);

const highlightJsAdapter = createHighlightJsAdapter(hljs);

const queryClient = new QueryClient();

// MAIA: only Query is available in Maia — all Prometheus-specific pages disabled
const mainNavPages = [
  {
    title: "Query",
    path: "/query",
    icon: <IconSearch style={navIconStyle} />,
    element: <QueryPage />,
    inAgentMode: false,
  },
];

const theme = createTheme({
  colors: {
    "codebox-bg": [
      "#f5f5f5",
      "#e7e7e7",
      "#cdcdcd",
      "#b2b2b2",
      "#9a9a9a",
      "#8b8b8b",
      "#848484",
      "#717171",
      "#656565",
      "#575757",
    ],
  },
});

const navLinkXPadding = "md";

function App() {
  const [opened, { toggle }] = useDisclosure();
  const { agentMode, consolesLink, pathPrefix } = useSettings();

  const navLinks = (
    <>
      {consolesLink && (
        <Button
          component="a"
          href={consolesLink}
          className={classes.link}
          leftSection={<IconDeviceDesktopAnalytics style={navIconStyle} />}
          px={navLinkXPadding}
        >
          Consoles
        </Button>
      )}
      {mainNavPages
        .filter((p) => !agentMode || p.inAgentMode)
        .map((p) => (
          <Button
            key={p.path}
            component={NavLink}
            to={p.path}
            className={classes.link}
            leftSection={p.icon}
            px={navLinkXPadding}
          >
            {p.title}
          </Button>
        ))}
    </>
  );

  const navActionIcons = (
    <>
      <ThemeSelector />
      {/* MAIA: disabled — NotificationsIcon triggers /api/v1/notifications/live SSE */}
      <SettingsMenu />
    </>
  );

  return (
    <BrowserRouter basename={pathPrefix}>
      <QueryParamProvider adapter={ReactRouter6Adapter}>
        <MantineProvider defaultColorScheme="auto" theme={theme}>
          <CodeHighlightAdapterProvider adapter={highlightJsAdapter}>
            <Notifications position="top-right" />
            <QueryClientProvider client={queryClient}>
              {/* MAIA: wrap with project provider so all child components can access current project */}
              <MaiaProjectProvider>
                <AppShell
                  header={{ height: 56 }}
                  navbar={{
                    width: 300,
                    breakpoint: "sm",
                    collapsed: { desktop: true, mobile: !opened },
                  }}
                  padding="md"
                >
                  {/* MAIA: NotificationsProvider removed — it opens /api/v1/notifications/live SSE */}
                  <AppShell.Header bg="rgb(65, 73, 81)" c="#fff">
                      <Group h="100%" px="md" wrap="nowrap">
                        <Group
                          style={{ flex: 1 }}
                          justify="space-between"
                          wrap="nowrap"
                        >
                          <Group gap={40} wrap="nowrap">
                            <Link
                              to="/"
                              style={{ textDecoration: "none", color: "white" }}
                            >
                              {/* MAIA: replaced Prometheus logo+text with Maia wordmark */}
                              <Text fz={22} fw={700} style={{ letterSpacing: 1 }}>
                                Maia
                              </Text>
                            </Link>
                            <Group gap={12} visibleFrom="sm" wrap="nowrap">
                              {navLinks}
                            </Group>
                          </Group>
                          <Group visibleFrom="xs" wrap="nowrap" gap="xs">
                            {/* MAIA: project switcher + classic UI link */}
                            <MaiaProjectSwitcher />
                            {navActionIcons}
                          </Group>
                        </Group>
                        <Burger
                          opened={opened}
                          onClick={toggle}
                          hiddenFrom="sm"
                          size="sm"
                          color="gray.2"
                        />
                      </Group>
                    </AppShell.Header>

                    <AppShell.Navbar py="md" px={4} bg="rgb(65, 73, 81)" c="#fff">
                      {navLinks}
                      <Group mt="md" hiddenFrom="xs" justify="center">
                        {navActionIcons}
                      </Group>
                    </AppShell.Navbar>

                  <AppShell.Main>
                    <ErrorBoundary key={location.pathname}>
                      <Suspense
                        fallback={
                          <Box mt="lg">
                            {Array.from(Array(10), (_, i) => (
                              <Skeleton
                                key={i}
                                height={40}
                                mb={15}
                                width={1000}
                                mx="auto"
                              />
                            ))}
                          </Box>
                        }
                      >
                        <Routes>
                          <Route
                            path="/"
                            element={
                              <Navigate
                                to={agentMode ? "/agent" : "/query"}
                                replace
                              />
                            }
                          />
                          {agentMode ? (
                            <Route
                              path="/agent"
                              element={
                                <ReadinessWrapper>
                                  <AgentPage />
                                </ReadinessWrapper>
                              }
                            />
                          ) : (
                            <Route
                              path="/query"
                              element={
                                <ReadinessWrapper>
                                  <QueryPage />
                                </ReadinessWrapper>
                              }
                            />
                          )}
                          {/* MAIA: redirect all Prometheus-only paths to /query */}
                          <Route path="/alerts" element={<Navigate to="/query" replace />} />
                          <Route path="/rules" element={<Navigate to="/query" replace />} />
                          <Route path="/targets" element={<Navigate to="/query" replace />} />
                          <Route path="/service-discovery" element={<Navigate to="/query" replace />} />
                          <Route path="/status" element={<Navigate to="/query" replace />} />
                          <Route path="/tsdb-status" element={<Navigate to="/query" replace />} />
                          <Route path="/flags" element={<Navigate to="/query" replace />} />
                          <Route path="/config" element={<Navigate to="/query" replace />} />
                          <Route path="/alertmanager-discovery" element={<Navigate to="/query" replace />} />
                        </Routes>
                      </Suspense>
                    </ErrorBoundary>
                  </AppShell.Main>
                </AppShell>
              </MaiaProjectProvider>
            </QueryClientProvider>
          </CodeHighlightAdapterProvider>
        </MantineProvider>
      </QueryParamProvider>
    </BrowserRouter>
  );
}

export default App;
