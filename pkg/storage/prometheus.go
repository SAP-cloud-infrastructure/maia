// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"net/http"

	"net/url"

	"fmt"
	"io"

	"github.com/spf13/viper"

	"github.com/sapcc/go-bits/logg"
)

type prometheusStorageClient struct {
	httpClient       *http.Client
	url, federateURL *url.URL
	customHeaders    map[string]string
}

// Prometheus creates a storage driver for Prometheus/Maia
func Prometheus(prometheusAPIURL string, customHeaders map[string]string) Driver {
	parsedURL, err := url.Parse(prometheusAPIURL)
	if err != nil {
		panic(err)
	}
	result := prometheusStorageClient{
		url:           parsedURL,
		customHeaders: customHeaders,
	}
	result.init()
	return &result
}

func (promCli *prometheusStorageClient) init() {
	if viper.IsSet("maia.proxy") {
		proxyURL, err := url.Parse(viper.GetString("maia.proxy"))
		if err != nil {
			panic(fmt.Errorf("could not set proxy: %s .\n%s", proxyURL, err.Error()))
		} else {
			promCli.httpClient = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
			return
		}
	}
	promCli.httpClient = &http.Client{}

	// if federateURL is configured, this will direct /federate requests to another host URL
	if viper.IsSet("maia.federate_url") {
		parsedURL, err := url.Parse(viper.GetString("maia.federate_url"))
		if err != nil {
			panic(err)
		}
		promCli.federateURL = parsedURL
	} else {
		promCli.federateURL = promCli.url
	}
}

func (promCli *prometheusStorageClient) Query(query, time, timeout, acceptContentType string) (*http.Response, error) {
	promURL := promCli.buildURL("/api/v1/query", map[string]any{"query": query, "time": time, "timeout": timeout})

	return promCli.sendToPrometheus("GET", promURL.String(), nil, map[string]string{"Accept": acceptContentType})
}

func (promCli *prometheusStorageClient) QueryRange(query, start, end, step, timeout, acceptContentType string) (*http.Response, error) {
	promURL := promCli.buildURL("/api/v1/query_range", map[string]any{"query": query, "start": start, "end": end,
		"step": step, "timeout": timeout})

	return promCli.sendToPrometheus("GET", promURL.String(), nil, map[string]string{"Accept": acceptContentType})
}

func (promCli *prometheusStorageClient) Series(match []string, start, end, acceptContentType string) (*http.Response, error) {
	promURL := promCli.buildURL("/api/v1/series", map[string]any{"match[]": match, "start": start, "end": end})

	return promCli.sendToPrometheus("GET", promURL.String(), nil, map[string]string{"Accept": acceptContentType})
}

func (promCli *prometheusStorageClient) LabelValues(name, acceptContentType string) (*http.Response, error) {
	promURL := promCli.buildURL("/api/v1/label/"+name+"/values", map[string]any{})

	res, err := promCli.sendToPrometheus("GET", promURL.String(), nil, map[string]string{"Accept": acceptContentType})

	return res, err
}

// LabelNames returns all label names that are used in the time series data ingested by the Prometheus instance.
// https://prometheus.io/docs/prometheus/latest/querying/api/#getting-label-names
// match[]=<series_selector>: Repeated series selector argument that selects the series to return. At least one match[] argument must be provided.
// Does this mean we need to use /api/v1/series to get the series selector?
func (promCli *prometheusStorageClient) Labels(start, end string, match []string, acceptContentType string) (*http.Response, error) {
	promURL := promCli.buildURL("/api/v1/labels", map[string]any{"start": start, "end": end, "match[]": match})

	return promCli.sendToPrometheus("GET", promURL.String(), nil, map[string]string{"Accept": acceptContentType})
}

func (promCli *prometheusStorageClient) Federate(selectors []string, acceptContentType string) (*http.Response, error) {
	promURL := promCli.buildURL("/federate", map[string]any{"match[]": selectors})

	return promCli.sendToPrometheus("GET", promURL.String(), nil, map[string]string{"Accept": acceptContentType})
}

func (promCli *prometheusStorageClient) DelegateRequest(request *http.Request) (*http.Response, error) {
	promURL := promCli.mapURL(request.URL)

	return promCli.sendToPrometheus(request.Method, promURL.String(), request.Body, map[string]string{"Accept": request.Header.Get("Accept")})
}

// buildURL is used to build the target URL of a Prometheus call
func (promCli *prometheusStorageClient) buildURL(path string, params map[string]any) url.URL {
	promURL := *promCli.url
	// treat federate special
	if path == "/federate" {
		promURL = *promCli.federateURL
	}

	// change original request to point to our backing Prometheus
	promURL.Path += path
	queryParams := url.Values{}
	for k, v := range params {
		if s, ok := v.(string); ok {
			if s != "" {
				queryParams.Add(k, s)
			}
		} else {
			for _, s := range v.([]string) {
				queryParams.Add(k, s)
			}
		}
	}
	promURL.RawQuery = queryParams.Encode()

	return promURL
}

// mapURL is used to map a Maia URL to Prometheus URL
func (promCli *prometheusStorageClient) mapURL(maiaURL *url.URL) url.URL {
	promURL := *maiaURL

	// change original request to point to our backing Prometheus
	promURL.Host = promCli.url.Host
	promURL.Scheme = promCli.url.Scheme
	promURL.User = promCli.url.User
	promURL.RawQuery = ""

	return promURL
}

// SendToPrometheus takes care of the request wrapping and delivery to Prometheus
func (promCli *prometheusStorageClient) sendToPrometheus(method, promURL string, body io.Reader, headers map[string]string) (*http.Response, error) {
	// Validate the URL before proceeding with the request.
	if !isValidURL(promURL) {
		return nil, fmt.Errorf("invalid URL: %s", promURL)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, promURL, body)
	if err != nil {
		logg.Error("Could not create request.\n", err.Error())
		return nil, err
	}

	for k, v := range promCli.customHeaders {
		req.Header.Add(k, v)
	}
	for k, v := range headers {
		req.Header.Add(k, v)
	}

	logg.Debug("Forwarding request to API: %s", promURL)

	resp, err := promCli.httpClient.Do(req)
	if err != nil {
		logg.Error("Request failed.\n%s", err.Error())
		return nil, err
	}
	return resp, nil
}

// isValidURL checks if the provided URL is well-formed and adheres to basic validation rules.
func isValidURL(urlStr string) bool {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Check if the scheme is http or https.
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	// Check if the host is non-empty.
	if parsedURL.Host == "" {
		return false
	}

	return true
}
