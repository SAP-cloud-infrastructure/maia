// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/prometheus/common/model"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/maia/pkg/keystone"
	"github.com/sapcc/maia/pkg/storage"
)

const (
	timestampKey = "__timestamp__"
	valueKey     = "__value__"
)

var maiaURL string
var selector string
var auth = gophercloud.AuthOptions{Scope: new(gophercloud.AuthScope)}
var authType string
var scopedDomain string
var outputFormat string
var jsonTemplate string
var columns string
var separator string
var starttime, endtime, timestamp string
var timeout, stepsize time.Duration

var keystoneDriver keystone.Driver
var storageDriver storage.Driver
var tzLocation = time.Local

// recoverAll is used to turn panics into error output
// we use panics here for any errors
func recoverAll() {
	if r := recover(); r != nil {
		fmt.Fprintln(os.Stderr, r)
	}
}

func fetchToken(ctx context.Context) {
	if scopedDomain != "" {
		auth.Scope.DomainName = scopedDomain
	}

	// expand those default values that are passed indirectly to avoid they are shown when calling maia --help
	if auth.Password == "$OS_PASSWORD" {
		auth.Password = os.Getenv("OS_PASSWORD")
	}
	if auth.TokenID == "$OS_TOKEN" {
		auth.TokenID = os.Getenv("OS_TOKEN")
	}
	if auth.ApplicationCredentialSecret == "$OS_APPLICATION_CREDENTIAL_SECRET" {
		auth.ApplicationCredentialSecret = os.Getenv("OS_APPLICATION_CREDENTIAL_SECRET")
	}

	// skip token creation and catalog lookup
	// if both token and Maia endpoint are defined
	if auth.TokenID != "" && maiaURL != "" {
		return
	}

	// default authType of all OpenStack clients is password
	if authType == "" {
		authType = "password"
		logg.Info("Authentication type defaults to %s", authType)
	}

	// ignore any parameters not related to the selected authentication type
	// to avoid keystone errors for the sake of consumability
	switch authType {
	case "password":
		// check mandatory stuff
		if auth.Password == "" {
			panic(errors.New("you must specify --os-password"))
		}
		if auth.Username == "" && auth.UserID == "" {
			panic(errors.New("you must specify --os-username or --os-user-id"))
		}
		// ignore tokens and application credentials
		auth.TokenID = ""
		auth.ApplicationCredentialName = ""
		auth.ApplicationCredentialID = ""
		auth.ApplicationCredentialSecret = ""
	case "token":
		// check mandatory stuff
		if auth.TokenID == "" {
			panic(errors.New("you must specify --os-token"))
		}
		// ignore anything but scope (to permit rescoping)
		auth.Password = ""
		auth.UserID = ""
		auth.Username = ""
		auth.DomainID = ""
		auth.DomainName = ""
		auth.ApplicationCredentialName = ""
		auth.ApplicationCredentialID = ""
		auth.ApplicationCredentialSecret = ""
	case "v3applicationcredential":
		// check mandatory stuff
		if auth.ApplicationCredentialSecret == "" {
			panic(errors.New("you must specify --os-application-credential-secret"))
		}
		if auth.ApplicationCredentialName != "" && auth.Username == "" && auth.UserID == "" {
			panic(errors.New("you must specify --os-username or --os-user-id when using" +
				" --os-application-credential-name"))
		}
		// ignore anything user identifiers if specified by application credential ID
		if auth.ApplicationCredentialID != "" {
			auth.UserID = ""
			auth.Username = ""
			auth.DomainID = ""
			auth.DomainName = ""
		}
		// ignore tokens, passwords and most notably scope
		auth.Password = ""
		auth.TokenID = ""
		auth.Scope = nil
	}

	// error on ambiguous parameters
	if auth.UserID != "" && auth.Username != "" {
		panic(errors.New("use either --os-user-id or --os-user-name but not both"))
	}
	if auth.DomainID != "" && auth.DomainName != "" {
		panic(errors.New("use either --os-user-domain-id or --os-user-domain-name but not both"))
	}
	if auth.UserID != "" && (auth.DomainID != "" || auth.DomainName != "") {
		panic(errors.New("do not specify --os-user-domain-id or --os-user-domain-name when using --os-user-id since the user ID implies the domain"))
	}

	// finally ... authenticate with keystone
	policyContext, url, err := keystoneInstance().Authenticate(ctx, auth)
	if err != nil {
		panic(err)
	}
	// keep the token and use the URL from the catalog (unless set explicitly)
	auth.TokenID = policyContext.Auth["token"]
	if maiaURL == "" {
		maiaURL = url
	}
}

// storageInstance creates a new Prometheus driver instance lazily
func storageInstance() storage.Driver {
	ctx := context.Background()
	if storageDriver == nil {
		switch {
		case promURL != "":
			// For direct Prometheus connections, prepare headers including global flag if needed
			headers := map[string]string{}
			if useGlobalKeystone {
				headers["X-Global-Region"] = "true"
			}
			storageDriver = storage.NewPrometheusDriver(promURL, headers)
		case auth.IdentityEndpoint != "":
			// authenticate and set maiaURL if missing
			fetchToken(ctx)
			// For Maia connections, prepare headers including auth token and global flag if needed
			headers := map[string]string{"X-Auth-Token": auth.TokenID}
			if useGlobalKeystone {
				headers["X-Global-Region"] = "true"
			}
			storageDriver = storage.NewPrometheusDriver(maiaURL, headers)
		default:
			panic(errors.New("either --os-auth-url or --prometheus-url need to be specified"))
		}
	}

	return storageDriver
}

// keystoneInstance creates a new keystone driver instance lazily
func keystoneInstance() keystone.Driver {
	if keystoneDriver == nil {
		setKeystoneInstance(keystone.NewKeystoneDriver())
	}
	return keystoneDriver
}

// printValues prints the result of a Maia API as raw values
//
//nolint:gocritic
func printValues(resp *http.Response) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(fmt.Errorf("server responsed with error code %d: %s", resp.StatusCode, err.Error()))
	} else {
		contentType := resp.Header.Get("Content-Type")
		if contentType == storage.JSON {
			if strings.EqualFold(outputFormat, "json") {
				fmt.Print(string(body))
			} else if strings.EqualFold(outputFormat, "value") {
				var jsonResponse struct {
					Value []string `json:"data,omitempty"`
				}
				if err := json.Unmarshal(body, &jsonResponse); err != nil {
					panic(err)
				}

				for _, value := range jsonResponse.Value {
					fmt.Println(value)
				}
			} else {
				panic(fmt.Errorf("unsupported --format value for this command: %s", outputFormat))
			}
		} else if strings.HasPrefix(contentType, "text/plain") {
			if strings.EqualFold(outputFormat, "value") {
				fmt.Print(string(body))
			} else {
				panic(fmt.Errorf("unsupported --format value for this command: %s", outputFormat))
			}
		} else {
			logg.Error("Response body: %s", string(body))
			panic(fmt.Errorf("unsupported response type from server: %s", contentType))
		}
	}
}

// printTable formats the result of a Maia API call as table
//
//nolint:gocritic
func printTable(resp *http.Response) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("server responsed with error code %d: %s", resp.StatusCode, err.Error())
	} else {
		contentType := resp.Header.Get("Content-Type")
		if contentType == storage.JSON {
			// JSON is not preprocessed
			if strings.EqualFold(outputFormat, "json") {
				fmt.Print(string(body))
				return
			} else if strings.EqualFold(outputFormat, "table") || strings.EqualFold(outputFormat, "value") {
				// unmarshal
				var jsonResponse struct {
					Table []model.LabelSet `json:"data,omitempty"`
				}
				if err := json.Unmarshal(body, &jsonResponse); err != nil {
					panic(err)
				}

				// determine relevant columns
				var allColumns []string
				if columns == "" {
					allColumns = extractSeriesColumns(jsonResponse.Table)
				} else {
					allColumns = strings.Split(columns, ",")
				}

				printHeader(allColumns)

				// Print relevant columns in sorted order
				for _, series := range jsonResponse.Table {
					row := map[string]string{}
					for k, v := range series {
						row[string(k)] = string(v)
					}
					printRow(allColumns, row)
				}
			} else {
				panic(fmt.Errorf("unsupported --format value for this command: %s", outputFormat))
			}
		} else if strings.HasPrefix(contentType, "text/plain") {
			// This affects /federate aka. metrics only. There is no point in filtering this output
			fmt.Print(string(body))
		} else {
			logg.Info("WARNING: Response body: %s", string(body))
			panic(fmt.Errorf("unsupported response type from server: %s", contentType))
		}
	}
}

// buildColumnSet determines which columns are contained in a Maia (Prometheus) query result
// if the columns have been explicitly set via `--columns`, only those
// columns will be taken into account regardless of the query result
func buildColumnSet(promResult model.Value) map[string]bool {
	result := map[string]bool{}
	if columns != "" {
		for c := range strings.SplitSeq(columns, ",") {
			result[c] = true
		}
	} else if vector, ok := promResult.(model.Vector); ok { //nolint:gocritic
		for _, el := range vector {
			collectKeys(result, model.LabelSet(el.Metric))
		}
	} else if matrix, ok := promResult.(model.Matrix); ok {
		for _, el := range matrix {
			collectKeys(result, model.LabelSet(el.Metric))
		}
	}
	return result
}

// printHeader prints the header row of a tabular output
func printHeader(allColumns []string) {
	if !strings.EqualFold(outputFormat, "value") {
		for i, field := range allColumns {
			if i > 0 {
				fmt.Print(separator)
			}
			fmt.Print(field)
		}
		fmt.Println()
	}
}

// extractSeriesColumns determines which columns (labels)
// are contained in a `series` API call.
func extractSeriesColumns(table []model.LabelSet) []string {
	// print all columns
	set := map[string]bool{}
	for _, rec := range table {
		collectKeys(set, rec)
	}
	return makeColumns(set)
}

func collectKeys(collector map[string]bool, input model.LabelSet) {
	// print all columns
	for label := range input {
		collector[string(label)] = true
	}
}

func makeColumns(collector map[string]bool) []string {
	// print all columns
	allColumns := []string{}
	for k := range collector {
		allColumns = append(allColumns, k)
	}
	sort.Strings(allColumns)
	return allColumns
}

func printRow(allColumns []string, rec map[string]string) {
	for i, field := range allColumns {
		if i > 0 {
			fmt.Print(separator)
		}
		if v, ok := rec[field]; ok {
			fmt.Print(v)
		}
	}
	fmt.Println()
}

func printTemplate(body []byte, tpl string) {
	t := template.Must(template.New("").Parse(tpl))
	m := map[string]any{}
	if err := json.Unmarshal(body, &m); err != nil {
		panic(err)
	}
	if err := t.Execute(os.Stdout, m); err != nil {
		panic(err)
	}
}

// timeColumnFromTS creates a table column for a timestamp; it rounds it off to the step-size
func timeColumnFromTS(ts time.Time) string {
	return ts.Truncate(stepsize).In(tzLocation).Format(time.RFC3339)
}

func printQueryResultAsTable(body []byte) {
	var queryResponse storage.QueryResponse
	err := json.Unmarshal(body, &queryResponse)
	if err != nil {
		panic(err)
	}

	valueObject := model.Value(queryResponse.Data.Value) //nolint:unconvert

	rows := []map[string]string{}
	var allColumns []string

	switch valueObject.Type() {
	case model.ValMatrix:
		matrix := valueObject.(model.Matrix)
		tsSet := map[string]bool{}
		// if no columns have been specified by user then collect them all
		set := buildColumnSet(matrix)
		for _, el := range matrix {
			columnValues := map[string]string{}
			for labelKey, labelValue := range el.Metric {
				columnValues[string(labelKey)] = string(labelValue)
			}
			for _, value := range el.Values {
				s := timeColumnFromTS(value.Timestamp.Time())
				tsSet[s] = true
				columnValues[s] = value.Value.String()
			}
			rows = append(rows, columnValues)
		}
		// have columns be set by user explicitly?
		if columns != "" {
			allColumns = strings.Split(columns, ",")
		} else {
			allColumns = makeColumns(set)
		}
		allColumns = append(allColumns, makeColumns(tsSet)...)
	case model.ValVector:
		matrix := valueObject.(model.Vector)
		set := buildColumnSet(matrix)
		for _, el := range matrix {
			collectKeys(set, model.LabelSet(el.Metric))
			columnValues := map[string]string{}
			columnValues[timestampKey] = el.Timestamp.Time().In(tzLocation).Format(time.RFC3339Nano)
			columnValues[valueKey] = el.Value.String()
			for labelKey, labelValue := range el.Metric {
				columnValues[string(labelKey)] = string(labelValue)
			}
			rows = append(rows, columnValues)
		}
		// have columns be set by user explicitly?
		if columns != "" {
			allColumns = strings.Split(columns, ",")
		} else {
			allColumns = makeColumns(set)
		}
		allColumns = append(allColumns, []string{timestampKey, valueKey}...)
	case model.ValScalar:
		scalarValue := valueObject.(*model.Scalar)
		allColumns = []string{timestampKey, valueKey}
		rows = []map[string]string{{timestampKey: scalarValue.Timestamp.Time().In(tzLocation).Format(time.RFC3339Nano), valueKey: scalarValue.String()}}
	}

	printHeader(allColumns)
	for _, row := range rows {
		printRow(allColumns, row)
	}
}

func printQueryResponse(resp *http.Response) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Server responsed with error code %d: %s", resp.StatusCode, err.Error())
		return
	}

	contentType := resp.Header.Get("Content-Type")
	switch contentType {
	case storage.JSON:
		switch strings.ToLower(outputFormat) {
		case "json":
			fmt.Print(string(body))
		case "template":
			if jsonTemplate == "" {
				panic(errors.New("missing --template parameter"))
			}
			printTemplate(body, jsonTemplate)
		case "table":
			printQueryResultAsTable(body)
		default:
			panic(fmt.Errorf("unsupported --format value for this command: %s", outputFormat))
		}
	default:
		logg.Info("WARNING: Response body: %s", string(body))
		panic(fmt.Errorf("unsupported response type from server: %s", contentType))
	}
}

// Snapshot is just public because unit testing frameworks complains otherwise
func Snapshot(cmd *cobra.Command, args []string) (ret error) {
	// transform panics with error params into errors
	defer recoverAll()

	setDefaultOutputFormat("value")

	prometheus := storageInstance()

	var resp *http.Response
	resp, err := prometheus.Federate([]string{"{" + selector + "}"}, storage.PlainText)
	checkResponse(err, resp)

	printValues(resp)

	return nil
}

// LabelValues is just public because unit testing frameworks complains otherwise
func LabelValues(cmd *cobra.Command, args []string) (ret error) {
	// transform panics with error params into errors
	defer recoverAll()

	setDefaultOutputFormat("value")

	// check parameters
	if len(args) < 1 {
		return errors.New("missing argument: label-name")
	}
	labelName := args[0]

	prometheus := storageInstance()

	var resp *http.Response
	resp, err := prometheus.LabelValues(labelName, storage.JSON)
	checkResponse(err, resp)

	printValues(resp)

	return nil
}

// Series is just public because unit testing frameworks complains otherwise
func Series(cmd *cobra.Command, args []string) (ret error) {
	// transform panics with error params into errors
	defer recoverAll()

	setDefaultOutputFormat("table")
	starttime, endtime = defaultTimeRangeStr(starttime, endtime)

	// pass the keystone token to Maia and ensure that the result is text
	prometheus := storageInstance()

	var resp *http.Response
	resp, err := prometheus.Series([]string{"{" + selector + "}"}, starttime, endtime, storage.JSON)
	checkResponse(err, resp)

	printTable(resp)

	return nil
}

// MetricNames is just public because unit testing frameworks complains otherwise
func MetricNames(cmd *cobra.Command, args []string) (ret error) {
	// transform panics with error params into errors
	defer recoverAll()

	setDefaultOutputFormat("value")

	return LabelValues(cmd, []string{"__name__"})
}

func parseTime(timestamp string) time.Time {
	t, err := time.Parse(time.RFC3339, timestamp) //no:errcheck
	if err != nil {
		t, _ = time.Parse(time.UnixDate, timestamp) //nolint:errcheck
	}
	return t
}

func defaultTimeRangeStr(start, end string) (string, string) { //nolint:gocritic
	s, e := start, end
	if e == "" {
		e = time.Now().Format(time.RFC3339)
	}
	if s == "" {
		s = parseTime(e).Add(-3 * time.Hour).Format(time.RFC3339)
	}

	return s, e
}

// Query is just public because unit testing frameworks complains otherwise
func Query(cmd *cobra.Command, args []string) (ret error) {
	// transform panics with error params into errors
	defer recoverAll()

	setDefaultOutputFormat("json")

	// check parameters
	if len(args) < 1 {
		return errors.New("missing argument: PromQL Query")
	}
	queryExpr := args[0]

	var timeoutStr, stepStr string
	if timeout > 0 {
		// workaround parsing issues
		timeoutStr = fmt.Sprintf("%ds", int(timeout.Seconds()))
	} else {
		timeoutStr = ""
	}
	if stepsize > 0 {
		stepStr = fmt.Sprintf("%ds", int(stepsize.Seconds()))
	} else {
		stepStr = ""
	}

	prometheus := storageInstance()

	// perform (range-)Query
	var resp *http.Response
	var err error
	if starttime != "" || endtime != "" {
		starttime, endtime = defaultTimeRangeStr(starttime, endtime)
		if stepStr == "" {
			// default to max. of 10 values when stepsize has not been defined
			sz := parseTime(endtime).Sub(parseTime(starttime)) / 10
			sizes := []time.Duration{
				15 * time.Second, 30 * time.Second, 60 * time.Second, 90 * time.Second,
				2 * time.Minute, 3 * time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 20 * time.Minute, 30 * time.Minute,
				1 * time.Hour, 2 * time.Hour, 3 * time.Hour, 8 * time.Hour, 12 * time.Hour, 24 * time.Hour,
				2 * 24 * time.Hour, 3 * 24 * time.Hour, 7 * 24 * time.Hour, 14 * 24 * time.Hour, 30 * 24 * time.Hour}
			for _, s := range sizes {
				if s > sz {
					sz = s
					break
				}
			}
			stepStr = fmt.Sprintf("%ds", int(sz.Seconds()))
		}
		resp, err = prometheus.QueryRange(queryExpr, starttime, endtime, stepStr, timeoutStr, storage.JSON)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
	} else {
		resp, err = prometheus.Query(queryExpr, timestamp, timeoutStr, storage.JSON)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
	}

	checkResponse(err, resp)

	printQueryResponse(resp)
	return nil
}

func setDefaultOutputFormat(format string) {
	if outputFormat == "" {
		outputFormat = format
	}
}

// checkHttpStatus checks whether the response is 200 and panics with an appropriate error otherwise
func checkResponse(err error, resp *http.Response) {
	if err != nil {
		panic(err)
	} else if resp.StatusCode != http.StatusOK {
		// Error handling for HTTP 503 (Service Unavailable)
		if resp.StatusCode == http.StatusServiceUnavailable {
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				// Include context about global flag if set
				if useGlobalKeystone {
					panic(fmt.Errorf("global keystone backend unavailable (HTTP %d) - failed to read response body: %w", resp.StatusCode, err))
				}
				panic(fmt.Errorf("service unavailable (HTTP %d) - failed to read response body: %w", resp.StatusCode, err))
			}
			if len(body) > 0 {
				// Include context about global flag if set
				if useGlobalKeystone {
					panic(fmt.Errorf("global keystone backend unavailable: %s", string(body)))
				}
				panic(fmt.Errorf("service unavailable: %s", string(body)))
			}
			// Fall back to generic message if no body
			if useGlobalKeystone {
				panic(fmt.Errorf("global keystone backend unavailable (HTTP %d)", resp.StatusCode))
			}
			panic(fmt.Errorf("service unavailable (HTTP %d)", resp.StatusCode))
		}
		panic(fmt.Errorf("server failed with status: %s (%d)", resp.Status, resp.StatusCode))
	}
}

var snapshotCmd = &cobra.Command{
	Use:   "snapshot [ --selector <vector-selector> ]",
	Short: "Get a Snapshot of the actual metric values for a project/domain.",
	Long:  "Displays the current values of all metric Series. The Series can filtered using vector-selectors (label constraints).",
	RunE:  Snapshot,
}

var seriesCmd = &cobra.Command{
	Use:   "series [ --selector <vector-selector> ] [ [ --start <starttime> ] [ --end <endtime> ] ]",
	Short: "List measurement Series for project/domain.",
	Long:  "Lists all metric Series. The Series can filtered using vector-selectors (label constraints).",
	RunE:  Series,
}

var labelValuesCmd = &cobra.Command{
	Use:   "label-values <label-name>",
	Short: "Get values for given label name.",
	Long:  "Obtains the possible values for a given label name (key) taking into account all Series that are currently stored.",
	RunE:  LabelValues,
}

var metricNamesCmd = &cobra.Command{
	Use:   "metric-names",
	Short: "Get list of metric names.",
	Long:  "Obtains a list of metric names taking into account all Series that are currently stored.",
	RunE:  MetricNames,
}

var queryCmd = &cobra.Command{
	Use:   "query <PromQL Query> [ --time | [ --start <starttime> ] [ --end <endtime> ] [ --step <duration> ] ] [ --timeout <duration> ]",
	Short: "Perform a PromQL Query",
	Long:  "Performs a PromQL query against the metrics available for the project/domain in scope",
	RunE:  Query,
}

func init() {
	RootCmd.AddCommand(snapshotCmd)
	RootCmd.AddCommand(queryCmd)
	RootCmd.AddCommand(seriesCmd)
	RootCmd.AddCommand(labelValuesCmd)
	RootCmd.AddCommand(metricNamesCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// snapshotCmd.PersistentFlags().String("foo", "", "A help for foo")

	// pass OpenStack auth. information via global top-level parameters or environment variables
	// it is used by the "serve" command as service user, otherwise to authenticate the client
	RootCmd.PersistentFlags().StringVar(&auth.IdentityEndpoint, "os-auth-url", os.Getenv("OS_AUTH_URL"), "OpenStack Authentication URL")
	err := viper.BindPFlag("keystone.auth_url", RootCmd.PersistentFlags().Lookup("os-auth-url"))
	if err != nil {
		panic(err)
	}

	RootCmd.PersistentFlags().StringVar(&auth.Username, "os-username", os.Getenv("OS_USERNAME"), "OpenStack Username")
	RootCmd.PersistentFlags().StringVar(&auth.UserID, "os-user-id", os.Getenv("OS_USER_ID"), "OpenStack Username")
	RootCmd.PersistentFlags().StringVar(&auth.Password, "os-password", "$OS_PASSWORD", "OpenStack Password") // avoid showing contents of $OS_PASSWORD as default value
	RootCmd.PersistentFlags().StringVar(&auth.DomainName, "os-user-domain-name", os.Getenv("OS_USER_DOMAIN_NAME"), "OpenStack User's domain name")
	RootCmd.PersistentFlags().StringVar(&auth.DomainID, "os-user-domain-id", os.Getenv("OS_USER_DOMAIN_ID"), "OpenStack User's domain ID")
	RootCmd.PersistentFlags().StringVar(&auth.Scope.ProjectName, "os-project-name", os.Getenv("OS_PROJECT_NAME"), "OpenStack Project name to scope to")
	RootCmd.PersistentFlags().StringVar(&auth.Scope.ProjectID, "os-project-id", os.Getenv("OS_PROJECT_ID"), "OpenStack Project ID to scope to")
	RootCmd.PersistentFlags().StringVar(&auth.Scope.DomainName, "os-project-domain-name", os.Getenv("OS_PROJECT_DOMAIN_NAME"), "OpenStack Project's domain name")
	RootCmd.PersistentFlags().StringVar(&scopedDomain, "os-domain-name", os.Getenv("OS_DOMAIN_NAME"), "OpenStack domain name to scope to")
	RootCmd.PersistentFlags().StringVar(&auth.Scope.DomainID, "os-domain-id", os.Getenv("OS_DOMAIN_ID"), "OpenStack domain ID to scope to")
	RootCmd.PersistentFlags().StringVar(&auth.TokenID, "os-token", "$OS_TOKEN", "OpenStack keystone token") // avoid showing contents of $OS_TOKEN as default value
	RootCmd.PersistentFlags().StringVar(&authType, "os-auth-type", os.Getenv("OS_AUTH_TYPE"), "OpenStack authentication type ('password' or 'token' or 'v3applicationcredential')")
	RootCmd.PersistentFlags().StringVar(&auth.ApplicationCredentialName, "os-application-credential-name", os.Getenv("OS_APPLICATION_CREDENTIAL_NAME"), "OpenStack application credential name")
	RootCmd.PersistentFlags().StringVar(&auth.ApplicationCredentialID, "os-application-credential-id", os.Getenv("OS_APPLICATION_CREDENTIAL_ID"), "OpenStack application credential id")
	RootCmd.PersistentFlags().StringVar(&auth.ApplicationCredentialSecret, "os-application-credential-secret", "$OS_APPLICATION_CREDENTIAL_SECRET", "OpenStack application credential secret") // avoid showing contents of $OS_PASSWORD as default value

	RootCmd.PersistentFlags().StringVarP(&outputFormat, "format", "f", "", "Specify output format: table, json, template or value")
	RootCmd.PersistentFlags().StringVarP(&columns, "columns", "c", "", "Specify the columns to print (comma-separated; only when --format value is set)")
	RootCmd.PersistentFlags().StringVar(&separator, "separator", " ", "Separate different columns with this string (only when --columns value is set; default <space>)")
	RootCmd.PersistentFlags().StringVar(&jsonTemplate, "template", "", "Go-template to define a custom output format based on the JSON response (only when --format=template)")

	RootCmd.PersistentFlags().StringVar(&maiaURL, "maia-url", os.Getenv("MAIA_URL"), "URL of the target Maia service (override OpenStack service catalog)")
	RootCmd.PersistentFlags().StringVar(&promURL, "prometheus-url", os.Getenv("MAIA_PROMETHEUS_URL"), "URL of the Prometheus server backing Maia (MAIA_PROMETHEUS_URL)")
	err = viper.BindPFlag("maia.prometheus_url", RootCmd.PersistentFlags().Lookup("prometheus-url"))
	if err != nil {
		panic(err)
	}

	snapshotCmd.Flags().StringVarP(&selector, "selector", "l", "", "Prometheus label-selector to restrict the amount of metrics")

	queryCmd.Flags().StringVar(&starttime, "start", "", "Range query: start timestamp (RFC3339 or Unix format; default: 3h before)")
	queryCmd.Flags().StringVar(&endtime, "end", "", "Range query: end timestamp (RFC3339 or Unix format; default: now)")
	queryCmd.Flags().StringVar(&timestamp, "time", "", "Instant query: timestamp of measurement (RFC3339 or Unix format; default: now)")
	queryCmd.Flags().DurationVarP(&timeout, "timeout", "", 0, "Optional: Timeout for Query (e.g. 10m; default: server setting)")
	queryCmd.Flags().DurationVarP(&stepsize, "step", "", 0, "Optional: Step size for range Query (e.g. 30s; default: sized to display 12 values)")

	seriesCmd.Flags().StringVarP(&selector, "selector", "l", "", "Prometheus label-selector to restrict the amount of metrics")
	seriesCmd.Flags().StringVar(&starttime, "start", "", "Start timestamp (RFC3339 or Unix format; default: 3h before)")
	seriesCmd.Flags().StringVar(&endtime, "end", "", "End timestamp (RFC3339 or Unix format; default: now)")
}

func setKeystoneInstance(driver keystone.Driver) {
	keystoneDriver = driver
}

func setStorageInstance(driver storage.Driver) {
	storageDriver = driver
}
