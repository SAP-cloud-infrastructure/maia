# Maia - AI Assistant Guide

## Project Overview

Maia is a multi-tenant OpenStack service that provides Prometheus metrics access with Keystone authentication. It acts as an authenticating proxy between OpenStack tenants and a Prometheus backend, injecting tenant constraints into all PromQL queries to enforce data isolation.

**Stack**: Go 1.26 · Prometheus · Keystone · cobra/viper · gorilla/mux · gophercloud · go-cache

## Quick Reference

```bash
make                     # Build binary to build/maia
make check               # Run all checks (tests + golangci-lint + typos)
make build/cover.out     # Tests with coverage
make build/cover.html    # HTML coverage report
make static-check        # All static analysis
make generate            # Code generation (must run before build)
make license-headers     # Add REUSE-compliant headers
```

## Architecture

```
Client → maia CLI → (Keystone auth) → Maia API → (scope injection) → Prometheus
```

Maia operates in three modes:
1. **Client mode**: CLI that authenticates via Keystone and queries a remote Maia service
2. **Standalone client**: CLI that queries Prometheus directly (no auth, `--prometheus-url`)
3. **Server mode**: HTTP service with Keystone auth, scope filtering, and Prometheus proxying

### Package Layout

| Package | Purpose |
|---------|---------|
| `main.go` | Entry point, signal handling (SIGINT/SIGTERM), context propagation |
| `pkg/cmd/` | CLI commands (cobra), flags, authentication mode selection, output formatting |
| `pkg/api/` | HTTP server, routing, middleware chain, authorization, scope filtering |
| `pkg/keystone/` | Keystone driver interface, token caching, project hierarchy, auth flows |
| `pkg/storage/` | Storage driver interface, Prometheus HTTP client (zero-copy proxy) |
| `pkg/util/` | PromQL AST modification for multi-tenancy (`promqlmod.go`) |
| `pkg/ui/` | Prometheus expression browser (embedded web assets via go-bindata) |
| `pkg/test/` | Test fixtures, HTTP response helpers, custom gomock matchers |

### Key Design Decisions

- **Zero-copy proxying**: Storage driver returns raw `*http.Response` from Prometheus — no unmarshal-marshal cycles. Maia modifies queries before sending, not responses after.
- **AST-based PromQL modification**: Uses Prometheus's parser to inject tenant constraints into the expression tree via visitor pattern (`labelInjector`). Never uses string manipulation on PromQL.
- **Context-based keystone resolution**: `keystoneResolutionMiddleware` determines regional vs global keystone once per request and stores it in `context.Context`. All downstream handlers retrieve via `getKeystoneFromContext()`. Eliminates race conditions.
- **Panic-based error handling in CLI**: Commands use `defer recoverAll()` to convert panics to error output. Validation failures panic immediately.
- **5-layer token caching**: Token cache (900s), project tree cache (900s), user projects cache (900s), user ID cache (24h), project scope cache (24h). All use go-cache (thread-safe).

## CLI Commands

```
maia
├── serve              # Run as server (reads /etc/maia/maia.conf)
├── snapshot           # Get metric snapshot (--selector/-l)
├── query              # Execute PromQL (--time for instant, --start/--end/--step for range)
├── series             # List metric series (--selector/-l, --start, --end)
├── label-values       # Get label values for a label name
└── metric-names       # List all metric names
```

### Authentication Mode Selection

The CLI selects auth mode automatically based on provided flags:

| Flags Provided | Mode |
|----------------|------|
| `--prometheus-url` | Direct Prometheus (no auth) |
| `--os-auth-url` | Keystone authentication → discover Maia from catalog |
| `--os-auth-type token` + `--os-token` | Token-based auth |
| `--os-auth-type v3applicationcredential` | Application credential auth |
| (default with `--os-auth-url`) | Password auth (`--os-username` + `--os-password`) |

All `OS_*` environment variables are supported (e.g., `OS_AUTH_URL`, `OS_USERNAME`, `OS_PASSWORD`).

### Output Formats (`--format/-f`)

| Format | Description | Default For |
|--------|-------------|-------------|
| `json` | Raw JSON | query |
| `table` | Aligned columns | series |
| `value` | Plain values, one per line | snapshot, label-values, metric-names |
| `template` | Custom Go template (`--template`) | (query only) |

## API Endpoints

| Method | Path | Auth Rule | Description |
|--------|------|-----------|-------------|
| GET | `/api/v1/query` | `metric:show` | Instant PromQL query |
| GET | `/api/v1/query_range` | `metric:show` | Range PromQL query |
| GET | `/api/v1/series` | `metric:list` | List time series |
| GET | `/api/v1/label/{name}/values` | `metric:list` | Label values |
| GET | `/api/v1/labels` | (none) | List label names |
| GET | `/federate` | `metric:show` | Prometheus federation endpoint |
| GET | `/{domain}/graph` | (basic auth) | Expression browser UI |
| GET | `/metrics` | (none) | Prometheus metrics scrape |

### Middleware Chain

Request flow: CORS → `keystoneResolutionMiddleware` → `authorize()` → handler → `observeDuration()` → `observeResponseSize()`

The `gaugeInflight` middleware wraps the entire router for concurrent request tracking.

## Multi-Tenancy

All tenant-aware endpoints inject scope constraints into queries before forwarding to Prometheus:

1. `X-Project-Id` header → fetch child projects recursively → inject `project_id=~"id1|id2|id3"`
2. `X-Domain-Id` header (fallback) → inject `domain_id="domainID"`

**Expression modification**: `sum(up{job="api"})` becomes `sum(up{job="api",project_id=~"p1|p2"})` via AST visitor.

**Selector modification**: `{job="api"}` becomes `{job="api",project_id=~"p1|p2"}` via matcher append.

## Global Flag

The `--global` flag (client) or `?global=true` param / `X-Global-Region: true` header (server) selects the global keystone backend instead of regional. Priority: URL param > header > default (false). Accepted values: `true/1/yes/on` and `false/0/no/off`.

## Code Conventions

### Build System

- **go-makefile-maker** generates `Makefile`, `.golangci.yaml`, CI workflows, and `.typos.toml` from `Makefile.maker.yaml`
- Do NOT edit `Makefile`, `.golangci.yaml`, `.typos.toml`, or CI YAML directly — edit `Makefile.maker.yaml` and run `go-makefile-maker`
- Code generation (`make generate`) must run before build: mockgen + go-bindata + addlicense

### Linting

- golangci-lint v2 with 40+ linters (see `.golangci.yaml`)
- Import ordering: stdlib, third-party, `github.com/sapcc/maia` (enforced by goimports)
- typos spell checker configured in `Makefile.maker.yaml` (excludes `web/static/vendor/` and `docs/*.svg`)

### Licensing

- Apache-2.0 with REUSE compliance
- All source files must have SPDX headers
- Run `make license-headers` before committing new files

### Logging

- Uses `logg.Debug()`, `logg.Info()`, `logg.Error()` from go-bits
- Debug logging enabled via `MAIA_DEBUG=1`
- Debug prefixes: `[CHILD_PROJECTS_DEBUG]`, `[KEYSTONE_DEBUG]`, `[SCOPE_DEBUG]`

## Testing

### Running Tests

```bash
make check               # Full check suite (tests + lint)
make build/cover.out     # Tests only, with coverage
```

### Test Patterns

- **Table-driven tests**: See `pkg/cmd/cmd_test.go` `Test_Auth()` and `pkg/api/keystone_middleware_test.go` `TestEarlyKeystoneResolution()`
- **Example tests**: `ExampleSnapshot()`, `ExampleQuery_table()` etc. in `pkg/cmd/cmd_test.go` with `// Output:` validation
- **Mock drivers**: `gomock`-generated mocks for `storage.Driver` and `keystone.Driver`
- **HTTP mocking**: `gock` library for Prometheus/Keystone HTTP interactions
- **Test fixtures**: JSON/TXT files in `pkg/*/fixtures/` loaded via `test.HTTPResponseFromFile()`
- **Custom matchers**: `test.HTTPRequestMatcher` (validates + injects headers), `test.ContextMatcher`
- **Diff-based validation**: Expected vs actual output compared with `diff -u`, `.actual` files generated

### Adding Tests

1. Add fixture files to `pkg/<package>/fixtures/`
2. Use `gomock` for driver-level mocking or `gock` for HTTP-level mocking
3. For API tests, use `test.APIRequest{}.Check(t, router)` pattern
4. For CLI tests, use `Example` functions with `// Output:` comments

## Configuration

### Server Config (TOML)

Default: `/etc/maia/maia.conf`

```toml
[maia]
prometheus_url = "http://prometheus:9090"
bind_address = "0.0.0.0:9091"
label_value_ttl = "72h"

[keystone]
auth_url = "https://regional-keystone/v3/"
username = "maia"
password = "password"
roles = "monitoring_admin,monitoring_viewer"
token_cache_time = "900s"
policy_file = "etc/policy.json"

[keystone.global]                  # Optional global keystone
auth_url = "https://global-keystone/v3/"
username = "maia"
password = "globalpassword"
```

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `MAIA_DEBUG=1` | Enable debug logging |
| `MAIA_PROMETHEUS_URL` | Direct Prometheus URL (client mode) |
| `MAIA_URL` | Override Maia service URL from catalog |
| `MAIA_INSECURE=1` | Disable TLS verification (dev only) |
| `OS_AUTH_URL` | Keystone auth endpoint |
| `OS_USERNAME`, `OS_PASSWORD` | Keystone credentials |
| `OS_PROJECT_NAME`, `OS_PROJECT_ID` | Project scoping |
| `OS_DOMAIN_NAME`, `OS_DOMAIN_ID` | Domain scoping |
| `OS_TOKEN` | Pre-existing auth token |
| `OS_AUTH_TYPE` | Auth type (`password`, `token`, `v3applicationcredential`) |

## Prometheus Metrics

| Type | Metric | Labels |
|------|--------|--------|
| Gauge | `maia_requests_inflight` | — |
| Summary | `maia_request_duration_seconds` | `handler` |
| Summary | `maia_response_size_bytes` | `handler` |
| Counter | `maia_logon_errors_count` | — |
| Counter | `maia_logon_failures_count` | — |
| Counter | `maia_tsdb_errors_count` | — |

## Common Pitfalls

- **Don't edit Makefile, .golangci.yaml, .typos.toml, or CI workflows** — they are generated from `Makefile.maker.yaml`. Run `go-makefile-maker` to regenerate.
- **Run `make generate` before building** — mockgen and go-bindata outputs are required. Build will fail without them.
- **Scope headers are mandatory** — API endpoints panic if both `X-Project-Id` and `X-Domain-Id` are missing. This is intentional (indicates configuration error).
- **Cache keys include keystone context** — Regional and global keystone tokens are cached separately via `CTX:regional` / `CTX:global` suffix. Never share cache entries between contexts.
- **PromQL modification uses AST, not strings** — Always use `util.AddLabelConstraintToExpression()` or `util.AddLabelConstraintToSelector()`. Never manipulate PromQL strings directly.
- **go-bindata embeds the web UI** — The `pkg/ui/bindata.go` file is generated. If you modify web assets in `web/`, regenerate with `make generate`.
- **LabelValues uses a synthetic query** — The `/label/{name}/values` endpoint constructs `count({name!=""}) BY (name)` and queries a time range (`label_value_ttl`), not the native Prometheus label API.
