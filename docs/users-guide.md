<!--
SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company

SPDX-License-Identifier: Apache-2.0
-->

# Maia Users Guide

**Table of Contents**

* [Quick Start](#quick-start) - Get started in 5 minutes
* [Maia UI](#using-the-maia-ui) - Prometheus expression browser
* [Maia CLI](#using-the-maia-client) - Command-line interface
* [Troubleshooting](#troubleshooting) - Common issues and solutions

## Quick Start

Get started with Maia CLI in 5 minutes. This guide helps you run your first query against the Maia metrics service.

### Prerequisites

Before using Maia CLI, ensure you have:

1. **OpenStack Account**: Access to an OpenStack environment with Maia deployed
2. **Project Access**: Membership in at least one OpenStack project
3. **Required Role**: The `monitoring_viewer` or `monitoring_admin` role on your project
4. **Maia Binary**: Download from [GitHub releases](https://github.com/sapcc/maia/releases) or build from source

**Check if you have the required role**:
```bash
openstack role assignment list --user <your-username> --project <your-project>
```

**Check if Maia is available in your region**:
```bash
openstack catalog list | grep maia
# Or check endpoints directly
openstack endpoint list --service maia
```

### Step 1: Set Up Authentication

Maia uses standard OpenStack authentication. Set these environment variables with your credentials:

```bash
# Required: Identity service endpoint
export OS_AUTH_URL="https://identity.myregion.cloud.sap/v3"

# Required: Your credentials
export OS_USERNAME="myusername"
export OS_PASSWORD="mypassword"

# Required: Project scope
export OS_PROJECT_NAME="myproject"
export OS_PROJECT_DOMAIN_NAME="mydomain"

# Required: User domain (often same as project domain)
export OS_USER_DOMAIN_NAME="mydomain"

# Optional but recommended: Specify API version
export OS_IDENTITY_API_VERSION=3
```

**Verify authentication works**:
```bash
# This should succeed if credentials are correct
openstack token issue
```

### Step 2: List Available Metrics

Once authenticated, query Maia to see what metrics are available:

```bash
maia metric-names
```

**Expected output**: A list of metric names like:
```
openstack_compute_instances_gauge
openstack_compute_stuck_instances_count_gauge
limes_project_quota
prometheus_http_requests_total
...
```

**If this fails**, see the [Troubleshooting](#troubleshooting) section below.

### Step 3: Query a Metric

Run a simple PromQL query to get current metric values:

```bash
# Query which services are up
maia query 'up'

# Query OpenStack compute instances
maia query 'openstack_compute_instances_gauge'

# Get results as a formatted table
maia query 'up' --format table
```

### Step 4: Explore Your Data

```bash
# List all time series for a metric
maia series --selector='openstack_compute_instances_gauge'

# Get current snapshot of all metrics
maia snapshot

# Find all values for a label
maia label-values job
```

### Next Steps

- Read the [Maia CLI](#using-the-maia-client) section for detailed command documentation
- Learn about [Global Region Support](#global-region-support) for multi-region queries
- Explore [Output Formatting](#output-formatting) options for automation
- Set up [Grafana integration](#using-maia-with-grafana) for dashboards

### Performance Tip: Use Token Authentication

For better performance when making multiple queries, generate a token once and reuse it:

```bash
# Generate token (valid for ~1 hour by default)
export OS_TOKEN=$(openstack token issue -c id -f value)

# Now queries are faster (no repeated authentication)
maia metric-names
maia query 'up'
```

---

## Using the Maia UI

Maia comes with a [PromQL Expression Browser](https://prometheus.io/docs/visualization/browser/) borrowed from
Prometheus.

You can use it to discover metrics, series and perform ad-hoc queries leveraging all of PromQL's rich query syntax.

### Login

Just log-on using your OpenStack credentials.

```
URL: https://maia.myopenstack.net/myUserDomain
Username: myUser
Password: ********
```

Maia will choose a project for you. You can switch to any other project via the dropdown menu on the top-right side.

Instead of adding the name of the user-domain (e.g. `myUserDomain`) to the URL, you may also specify it as part of
the username, when the browser prompts for your credentials.

```
Username: myUser@myUserDomain
Password: ******
```

If you neither specify the user-domain in the username nor the URL, Maia will assume that the user is part
of the configured default domain (not to be confused with the OpenStack domain `default`).

```
URL: https://maia.myopenstack.net
Username: myUser
```

You may also use the special username syntax described in more detail [here](#openstack-authentication-and-authorization)
to log right into your target project.

```
Username: myuser@mydomain|myproject@mydomain
Password: ********
```

Or you use OpenStack _application credentials_:

```
# this is an example of ID-based login
username: *myappcredid
password: myappcredsecret
# this is an example of name-based login
username: *myappcredname@myuser@mydomain
password: myappcredsecret
```

### The Maia Screen

The Maia screen consists of three part:

* Navigation area (top)
* PromQL query input field with metrics dropdown list
* Result area with two tabs:
  - Graph area for visualizing the query result
  - Console area listing the different _series_ produced by the query

### Discover Metrics

You can use the dropdown list or the auto-completion functionality of the PromQL input field to discover which
metrics are known by the system.

```
openstack_compute_instances_gauge
```

Once you hit `<enter>`, Maia will provide you a list of all known time series for that metric in the `Console` area.

Usually this list is quite long. So you should restrict your query further by adding constraints about the labels
in curly braces. In Prometheus terminology, these constraints are called _selectors_

```
openstack_compute_instances_gauge{vm_state="active"}
```

### Visualize Series

Once you have restricted the number of series to a feasible amount, you may go ahead and graph them.

For that you just click on the `Graph` tab left from the `Console` one.

The displayed line graph shows the historical metric values within the selected timeframe.

You can use the following controls to adjust the graph:

* `-`/`+` can be used to reduce/extend the timeframe

* `<<`/`>>`can be used to shift the timeframe back resp. forth in time

* `Res. (s)` can be used to change the resolution i.e. adjust the size of a data point in seconds (e.g. enter `300s`
to get one cumulative value for each 5 minute interval)

## Using the Maia Client

The `maia` command can also be used to retrieve metrics from the Maia service. It behaves like any other OpenStack
 CLI, supporting the same command line options and environment variables for authentication:

| Option | Environment Variable | Description |
|:--------:|:----------------------:|:-------------:|
| --os-username | OS_USERNAME | OpenStack username, requires `os-user-domain-name` |
| --os-user-id | OS_USER_ID | OpenStack user unique ID |
| --os-password | OS_PASSWORD | Password |
| --os-token | OS_TOKEN | Pregenerated Keystone token with authorization scope |
| --os-application-credential-id | OS_APPLICATION_CREDENTIAL_ID | ID of an _application credential_ |
| --os-application-credential-name | OS_APPLICATION_CREDENTIAL_NAME | name of an _application credential_, scoped by user |
| --os-application-credential-secret | OS_APPLICATION_CREDENTIAL_SECRET | secret of an _application credential_ |
| --os-user-domain-name | OS_USER_DOMAIN_NAME | domain name, qualifying the username (default: `Default`) |
| --os-user-domain-id | OS_USER_DOMAIN_ID | domain unique ID, qualifying the username (default: `default`) |
| --os-project-name | OS_PROJECT_NAME | OpenStack project name for authorization scoping to project, requires `os-project-domain-name` |
| --os-project-id | OS_PROJECT_ID | OpenStack project unique ID |
| --os-domain-name | OS_DOMAIN_NAME | OpenStack domain name for authorization scoping to domain |
| --os-domain-id | OS_DOMAIN_ID | OpenStack domain unique ID for authorization scoping to domain |
| --os-auth-url | OS_AUTH_URL | Endpoint of the Identity v3 service. Needed to authentication and Maia endpoint lookup |
| --global | - | Use global keystone backend for metrics queries |
| --os-auth-type | OS_AUTH_TYPE | Authentication method to use: one of `password`, `token`, `v3applicationcredential`|

Usually, you can reuse your existing RC-files. For performance reasons, you should consider token-based
authentication whenever you make several calls to the Maia CLI.

Use `openstack token issue` to generate a token and pass it to the Maia CLI in the `OS_TOKEN` variable.

```
export OS_TOKEN=$(openstack token issue -c id -f value)
```

If for some reason you want to use another Maia endpoint than the one registered in the OpenStack service catalog,
then you can override its URL using the `--maia-url` option:

| Option | Environment Variable | Description |
|:--------:|:----------------------:|:-------------:|
| --maia-url | MAIA_URL | URL of the Maia service endpoint |

In the examples below we assume that you have initialized the OS_* variables your shell environment properly and that
your user has the prerequisite roles (e.g. `monitoring_viewer`) on the project in scope.

Type `maia --help` to get a full list of commands and options options with documentation.

```
maia --help
```

### Show Known Measurement Series

Use the `series` command to get a list of all measurement series. You can restrict the timeframe using
the parameters `--start` and `--end`.

```
maia series --selector "__name__=~'vc.*'" --start '2017-07-26T10:46:25+02:00'
```

The list of series can be filtered using Prometheus label matchers. Don't forget to put it in quotes.
```
maia snapshot --selector 'job="endpoints"' ...
```

### List Known Metric Names

Use the `metric-names` command to obtain a list of metric names.

```
maia metric-names
```

### List Known Label Values

Use the `label-values` command to obtain known values for a given label.

```
maia label-values "job"
```

Note that stale series which did not receive measurements recently may not be considered for this list.

### Query Metrics with PromQL

Use the `query` command to perform an arbitrary [PromQL-query](https://prometheus.io/docs/querying/basics/) against Maia.
It returns a single entry for each series. This is called an _instant query_.

```
maia query 'vcenter_virtualDisk_totalWriteLatency_average{vmware_name:"win_cifs_13"}'
```

Older values can be obtained using the `--time` parameter.

```
maia query ... --time 2017-07-01T05:10:51.781Z
```

Finally you can extract all values during a given timeframe by specifying a start- and end-date with the `--start` resp.
`--end` parameters. This is called a _range query_.

You should also specify the resolution using the `--stepsize` parameter. Otherwise
Maia will choose defaults that may not always fit well. Timestamps can be specified in Unix or RC3339 format. Durations are
specified as numbers with a unit suffix, such as `30s`, `1.5h` or `2h45m`. Valid time units are `ns`, `us`,
`ms`, `s`, `m`, `h`.

```
maia query ... --start 2017-07-01T05:10:51.781Z --end 2017-07-01T09:10:51.781Z --stepsize 300s
```

Also be aware that due to the sheer amount of data, range query results usually do not fit the width of a terminal screen.
For that reason the default output format for _range queries_ is `json` and not `table`. Keep this in mind when you want to
do a CSV export to a spreadsheet.

Enter `maia query --help` for more options.

### Output Formatting

By default maia prints results as unformatted text. Series data is formatted in raw tables without column alignment.
Labels are used as columns (alphabetical sorting). There are three additional columns which do not refer
to labels:

| Column Name | Meaning |
|:-------------:|:---------:|
| \_\_name\_\_ | the metric name |
| \_\_timestamp\_\_ |  the timestamp of a measurement |
| \_\_value\_\_ | the value of a measurement |

To enable automation, also JSON, plain values output and Go text-templates
are supported.

The output is controlled via the parameters `--format`, `--columns`, `--separator`and `--template`.

| Format   | Description | Additional Options                                                              |
|:----------:|:-------------:|:---------------------------------------------------------------------------------:|
| table | text output in tabular form | `--columns`: selects which metric-labels are displayed as columns<br>`--separator`: defines how columns are separated        |
| value | output of plain values in lists or tables | like `table`                                          |
| json     | JSON output of Maia/Prometheus server. Contains additional status/error information. See [Prometheus API doc.](https://prometheus.io/docs/querying/api/#expression-query-result-formats) | none |
| template | Highly configurable output, applying [Go-templates](https://golang.org/pkg/text/template/) to the JSON response (see `json`format) | `--template`: Go-template expression |

### Exporting Snapshots

Use the `snapshot` command to get the latest values of all series in
[textual form](https://prometheus.io/docs/instrumenting/exposition_formats/).

```
maia snapshot
```

The amount of data can be restricted using Prometheus label matchers, i.e. constraints on label values:

```
maia snapshot --selector 'job="endpoints"' ...
```

If you want to preprocess/filter data further, you can e.g. use the [prom2json](https://github.com/prometheus/prom2json)
tool together with [jq](https://github.com/stedolan/jq).

### Global Region Support

The Maia CLI supports querying metrics from global/virtual regions using the `--global` flag. This flag signals the Maia server to use the global keystone backend instead of the regional one.

**How it works**: The CLI uses the same environment variables (OS_USERNAME, OS_PASSWORD, etc.) to authenticate with the Maia service, regardless of whether you're querying regional or global metrics. The `--global` flag tells the Maia server which backend (regional or global keystone) to use for validating your credentials and retrieving metrics. The server handles the routing between its configured regional and global keystone instances.

#### Examples

```bash
# Set up standard authentication (same for both regional and global)
export OS_AUTH_URL="https://identity.myopenstack.net/v3/"
export OS_USERNAME="myuser"
export OS_PASSWORD="mypassword"
export OS_PROJECT_NAME="myproject"
export OS_PROJECT_DOMAIN_NAME="mydomain"

# Query metrics from the regional backend (default)
maia query "up"
maia series --selector="job=prometheus"
maia snapshot

# Query metrics from the global backend
maia query "up" --global
maia series --selector="job=prometheus" --global
maia snapshot --global

# Compare metrics between regional and global backends
echo "Regional metrics:"
maia metric-names | wc -l

echo "Global metrics:"
maia metric-names --global | wc -l

# Get specific label values from global backend
maia label-values instance --global
```

#### Error Handling

If the Maia server is not configured with global keystone support, you will receive an error:

```
Error: global keystone backend unavailable: global keystone requested but not configured
```

This typically means the server needs to be configured with a `[keystone.global]` section in its configuration file.

### Use Maia Client with Prometheus

You can also use the maia client with a plain Prometheus (no authentication).

```
maia snapshot --prometheus-url http://localhost:9090
```

## Using Maia with Grafana

Due to its API-compatibility, the Prometheus data source in Grafana can be used for Maia as well. That means you can
build elaborate dashboards around Maia metrics with your existing Grafana installation. No additional plugins needed!

Configure the data source like with a regular Prometheus. Select `Basic Authentication` and enter the scoped
 user credentials.

There are several variants to express the project/domain scope:

Project scoped user:

* `user_id|project_id`
* `username@user_domain_name|project_id`
* `user_id|project_name@project_domain_name`
* `username@user_domain_name|project_name@project_domain_name`
* `username@user_domain_name|project_name` (if project_domain_name = user_domain_name)

Domain scoped user:

* `user_id|@domain_name`
* `user_name@user_domain_name|@domain_name`

Application Credential:

* `*app_cred_id`
* `*app_cred_name@user_id`
* `*app_cred_name@user_name@user_domain_name`

### OpenStack Authentication and Authorization

In addition to 'native' OpenStack authentication using Keystone tokens, Maia supports basic authentication in order
to support existing clients like Grafana and federated Prometheus.

The problem with basic authentication is that it lacks a standard way to express OpenStack domain information. Also there
 is no means to express OpenStack authorization scopes. Since neither Prometheus nor Grafana support adding custom
 header fields to the requests to Prometheus and thus Maia, we have to encode both the domain information and the authorization
 scope into the username.

 For the domain qualification, we could borrow "@" from e-mail. So when a user or a project is identified by name, you
  can add the domain in the form `username@domainname`.

 The authorization scope is separated from the qualified username with a vertical bar "|", splitting the username
 into a username and scope part: `user|scope`. Like with usernames, also the scoped project resp. domain can be
 denoted by name: `projectname@domainname`. To disambiguate scoping by project-id and domain-name, the domain is always prefixed
 with `@`.

Alternatively, OpenStack _application credentials_ can be used in place of username and password. With these credentials you are implicitly scoped
to a single project (or domain), so there is no need to supply scope information as before.

To tell Maia that the username and password fields are actually containing _application credentials_,
you put an asterisk (`*`) in front of the username value.

There are two ways to authenticate with application credentials:
* ID-based: Use the application credential ID as username
* Name-based: Use the application credential name and qualify it using the username or user ID

In both cases you use the _secret_ of the application credential as password.

# Federating Maia to Prometheus

To configure Prometheus to receive data from Maia, the following job configuration has to be applied.

In the `basic_auth` section a valid user id, project id and password, corresponding to your OpenStack User and Project,
has to be provided. For convenience you can always use the `user_name@user_domain_name` syntax instead of the technical IDs.

The user is required to have the `metric:show` permission.

```yaml
scrape_configs:

  # The job name is added as a label `job=<job_name>` to any timeseries scraped from this config.
  - job_name: 'maia'
    metrics_path: "/federate"
    basic_auth:
      # Corresponds to your OpenStack User and Project
    username: <user_name>@<user_domain_name>|<project_name>@<project_domain_name>  # or <user_id>|<project_id>
    password: <password>

    static_configs:
      - targets: ['maia.<region>.cloud.sap:443']

```

Prometheus' targets page ( Status -> Targets ) should the new job and the endpoint with `State UP`.
The `Error` column should be empty.
It might indicate a failed authorization (`401 Unauthorized`).

---

## Troubleshooting

This section covers common issues when using the Maia CLI and their solutions.

### Authentication Failures

#### Error: "You are not authorized to perform the requested action"

**Cause**: Your user account does not have the required `monitoring_viewer` or `monitoring_admin` role on the project.

**Solution**:

1. Check your current role assignments:
   ```bash
   openstack role assignment list --user $OS_USERNAME --project $OS_PROJECT_NAME
   ```

2. If the role is missing, contact your OpenStack administrator to request the `monitoring_viewer` role.

3. Verify the role was added:
   ```bash
   openstack role assignment list --user $OS_USERNAME --project $OS_PROJECT_NAME | grep monitoring
   ```

#### Error: "Authentication failed" or "401 Unauthorized"

**Cause**: Invalid credentials, wrong domain, or incorrect authentication parameters.

**Solution**:

1. Verify your credentials work with the OpenStack CLI:
   ```bash
   openstack token issue
   ```

2. Check that your domains are correct:
   ```bash
   # User domain and project domain must match your actual setup
   echo "User Domain: $OS_USER_DOMAIN_NAME"
   echo "Project Domain: $OS_PROJECT_DOMAIN_NAME"
   ```

3. Common domain issues:
   - If you're getting "default domain" errors, explicitly set domains:
     ```bash
     export OS_USER_DOMAIN_NAME="your-actual-domain"
     export OS_PROJECT_DOMAIN_NAME="your-actual-domain"
     ```
   - Domain names are case-sensitive

4. If using application credentials, ensure the format is correct:
   ```bash
   export OS_AUTH_TYPE=v3applicationcredential
   export OS_APPLICATION_CREDENTIAL_ID="your-app-cred-id"
   export OS_APPLICATION_CREDENTIAL_SECRET="your-app-cred-secret"
   ```

### Service Discovery Issues

#### Error: "Could not find Maia endpoint in service catalog"

**Cause**: Maia service is not registered in your region's OpenStack service catalog.

**Solution**:

1. Verify Maia is deployed in your region:
   ```bash
   openstack catalog list | grep -i maia
   openstack endpoint list --service maia
   ```

2. If Maia is not listed, it may not be deployed in your region. Contact your cloud operator.

3. **Workaround**: If you know the Maia URL, bypass service discovery:
   ```bash
   maia metric-names --maia-url="https://maia.myregion.cloud.sap:443"
   ```

4. To always use a specific URL, set the environment variable:
   ```bash
   export MAIA_URL="https://maia.myregion.cloud.sap:443"
   maia metric-names  # Now uses MAIA_URL automatically
   ```

### Connection Issues

#### Error: "Connection timeout" or "Connection refused"

**Cause**: Network connectivity issue or incorrect URL.

**Solution**:

1. Test basic connectivity to the Maia endpoint:
   ```bash
   # Replace with your actual region
   curl -I https://maia.myregion.cloud.sap:443
   ```

2. Verify the correct region URL:
   ```bash
   # List all available endpoints
   openstack endpoint list | grep maia
   ```

3. Test Keystone connectivity first:
   ```bash
   curl -I $OS_AUTH_URL
   # Should return HTTP 300 Multiple Choices
   ```

### Query Issues

#### Error: "No metrics returned" or empty results

**Cause**: Incorrect metric names, label selectors, or time range; or no metrics exist for your project.

**Solution**:

1. List all available metrics first:
   ```bash
   maia metric-names
   ```

2. Verify the metric exists and check its labels:
   ```bash
   maia series --selector='__name__="your_metric_name"'
   ```

3. Check if metrics are being collected for your project:
   ```bash
   # Query a universal metric that should always exist
   maia query 'up'
   ```

4. Verify your project has monitoring data:
   - Metrics are project-scoped by default
   - You'll only see metrics from resources in your current project
   - Switch projects if needed: `export OS_PROJECT_NAME="other-project"`

#### Error: "server failed with status: 503 Service Unavailable"

**Cause**: Maia service or backend Prometheus is temporarily unavailable.

**Solution**:

1. Wait a few minutes and retry - this may be a temporary issue

2. Contact your cloud operations team if the issue persists

### Global Region Errors

#### Error: "global keystone backend unavailable" (when using --global flag)

**Cause**: The Maia server is not configured with global keystone support, or the global backend is down.

**Solution**:

1. Verify your region supports global queries:
   - Not all Maia deployments have global keystone configured
   - Contact your cloud operator to confirm availability

2. If global support exists, this may be a temporary outage:
   - Try again in a few minutes
   - Check with cloud operations for status

**Note**: The global region contains data that is separate from regional backends. If global is not available, you cannot access global-specific metrics by querying regional backends individually.

### Performance Issues

#### Issue: Queries are slow or timing out

**Solution**:

1. Use token authentication instead of password authentication:
   ```bash
   export OS_TOKEN=$(openstack token issue -c id -f value)
   # Tokens are valid for ~1 hour and avoid repeated Keystone calls
   ```

2. Reduce query scope with label matchers:
   ```bash
   # Instead of querying all series
   maia series --selector='job="prometheus"'

   # Be specific with time ranges for range queries
   maia query 'up' --start='2025-09-30T00:00:00Z' --end='2025-09-30T01:00:00Z'
   ```

3. Use appropriate step sizes for range queries:
   ```bash
   # Larger step sizes = fewer data points = faster queries
   maia query 'up' --start='2025-09-29T00:00:00Z' --end='2025-09-30T00:00:00Z' --stepsize='5m'
   ```

### Debug Mode

Enable debug logging to see detailed information about what Maia is doing:

```bash
# Set debug environment variable
export MAIA_DEBUG=1

# Now run commands with verbose output
maia metric-names

# Output will include:
# - Authentication type being used
# - API server URL
# - Detailed error messages
```

### Getting Help

If you continue to experience issues:

1. **Check Maia version**: Ensure you're using a recent version
   ```bash
   maia --version
   ```

2. **Contact Support**:
   - Open an issue: https://github.com/SAP-cloud-infrastructure/maia/issues
   - Provide: Maia version, error message, sanitized command (remove passwords)
   - Include debug output: `MAIA_DEBUG=1 maia <command> 2>&1`

3. **Community Resources**:
   - Maia GitHub: https://github.com/SAP-cloud-infrastructure/maia
   - OpenStack documentation: https://docs.openstack.org
