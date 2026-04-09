<!--
SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company

SPDX-License-Identifier: Apache-2.0
-->

# Maia

[![CI](https://github.com/SAP-cloud-infrastructure/maia/actions/workflows/ci.yaml/badge.svg)](https://github.com/SAP-cloud-infrastructure/maia/actions/workflows/ci.yaml)

Maia is a multi-tenant OpenStack-service for accessing metrics and alarms collected through Prometheus. It offers 
a [Prometheus-compatible](https://prometheus.io/docs/querying/api/) API and supports federation.

At SAP we use it to share tenant-specific metrics from our Converged Cloud platform
with our users. For their convenience we included a CLI, so that metrics can be discovered and
retrieved from shell scripts.

If you don't use OpenStack, you can still use Maia CLI as a feature-complete shell client for Prometheus. 

## Features

- Multi-tenant Prometheus API offering isolation and resource restriction
- Supports both project and domain-based authentication from OpenStack
- **Global region support** for virtual region metric queries using global keystone authentication
- Fetches required service endpoints from Keystone service catalog
- JSON Web Token validation for the OpenStack Elektra UI

[Maia Service](docs/operators-guide.md)

* OpenStack Identity v3 authentication and authorization
* Project- and domain-level access control (scoping)
* Compatible to Grafana's Prometheus data source 
* Compatible to Prometheus API (read-only)
* Supports secure federation to additional Prometheus instances

[Maia UI](docs/users-guide.md#using-the-maia-ui)

* Prometheus expression browser adapted to Maia
* Browse projects and metrics
* Perform ad-hoc PromQL queries
* Graph metrics

[Maia CLI](docs/users-guide.md#using-the-maia-client)

* Feature-complete CLI supporting all API operations
* JSON and Go-template-based output for reliable automation
* Works with Prometheus, too (no OpenStack required)
* **Global region support** via `--global` flag for querying metrics from virtual/global regions

## Installation

### Binary Releases

Binary releases for Linux and MacOS can be downloaded from the GitHub [releases](https://github.com/SAP-cloud-infrastructure/maia/releases) page.

### Build from Source

Requires Go (see `go.mod` for the minimum version).

```bash
make generate   # Code generation (must run first)
make            # Build binary to build/maia
make install    # Install to /usr (or PREFIX=/some/path)
make docker     # Build Docker image
```

## Using Maia

Maia can be used via Web-UI or CLI. Enter `maia --help` to see a list of commands and options.

```bash
maia metric-names                          # List available metrics
maia query 'up'                            # Run a PromQL query
maia series --selector='job="endpoints"'   # List time series
maia query 'up' --global                   # Query global region metrics
```

Please refer to the [Maia user guide](./docs/users-guide.md) for detailed instructions, including authentication setup, output formatting, Grafana integration, and Prometheus federation.

## For Operators

Deploy Maia as a Kubernetes service using the [Maia Helm chart](https://github.com/SAP-cloud-infrastructure/helm-charts/tree/master/openstack/maia) (includes Maia, Prometheus, and Thanos).

The [operators guide](./docs/operators-guide.md) covers configuration, Keystone integration, global region setup, exporter requirements, and monitoring.

## For Developers / Contributors

- **API integration**: The [developers guide](./docs/developers-guide.md) describes the Maia API, authentication schemes, and how to build exporters.
- **Contributing**: See the [developers guide](./docs/developers-guide.md#contributing) for pull request guidelines and architecture overview.
- **Releasing**: See [RELEASE.md](./RELEASE.md) for the release process.

## Support, Feedback, Contributing

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/creating-an-issue). Contribution and feedback are encouraged and always welcome. For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](https://github.com/SAP-cloud-infrastructure/.github/blob/main/CONTRIBUTING.md).

## Security / Disclosure

If you find any bug that may be a security problem, please follow our instructions [in our security policy](https://github.com/SAP-cloud-infrastructure/.github/blob/main/SECURITY.md) on how to report it. Please do not create GitHub issues for security-related doubts or problems.

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/SAP-cloud-infrastructure/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright 2017-2025 SAP SE or an SAP affiliate company and maia contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP-cloud-infrastructure/maia).