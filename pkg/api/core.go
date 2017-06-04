/*******************************************************************************
*
* Copyright 2017 SAP SE
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"bytes"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/common/expfmt"
	"github.com/sapcc/maia/pkg/keystone"
	"github.com/sapcc/maia/pkg/storage"

	dto "github.com/prometheus/client_model/go"
)

const RFC822 = "Mon, 2 Jan 2006 15:04:05 GMT"

//versionData is used by version advertisement handlers.
type versionData struct {
	Status string            `json:"status"`
	ID     string            `json:"id"`
	Links  []versionLinkData `json:"links"`
}

//versionLinkData is used by version advertisement handlers, as part of the
//versionData struct.
type versionLinkData struct {
	URL      string `json:"href"`
	Relation string `json:"rel"`
	Type     string `json:"type,omitempty"`
}

type v1Provider struct {
	keystone    keystone.Driver
	storage     storage.Driver
	versionData versionData
}

//NewV1Router creates a http.Handler that serves the Maia v1 API.
//It also returns the versionData for this API version which is needed for the
//version advertisement on "GET /".
func NewV1Router(keystone keystone.Driver, storage storage.Driver) (http.Handler, versionData) {
	r := mux.NewRouter()
	p := &v1Provider{
		keystone: keystone,
		storage:  storage,
	}
	p.versionData = versionData{
		Status: "CURRENT",
		ID:     "v1",
		Links: []versionLinkData{
			{
				Relation: "self",
				URL:      p.Path(),
			},
			{
				Relation: "describedby",
				URL:      "https://github.com/sapcc/maia/tree/master/docs",
				Type:     "text/html",
			},
		},
	}

	r.Methods("GET").Path("/v1/").HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		ReturnJSON(res, 200, map[string]interface{}{"version": p.versionData})
	})

	r.Methods("GET").Path("/v1/metrics").HandlerFunc(p.ListMetrics)

	return r, p.versionData
}

// Content-Encoding gzip
// Content-Length
// Content-Type application/vnd.google.protobuf; proto=io.prometheus.client.MetricFamily; encoding=delimited
// Date
// Connection close

func ReturnMetrics(w http.ResponseWriter, format expfmt.Format, code int, data *dto.MetricFamily) {

	time := time.Now().UTC().Format(RFC822)
	enc := expfmt.NewEncoder(w, format)

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "application/vnd.google.protobuf; proto=io.prometheus.client.MetricFamily; encoding=delimited")
	//w.Header().Set("Content-Length", "")
	w.Header().Set("Date", time)
	w.Header().Set("Connection", "close")

	enc.Encode(data)

}

//ReturnJSON is a convenience function for HTTP handlers returning JSON data.
//The `code` argument specifies the HTTP response code, usually 200.
func ReturnJSON(w http.ResponseWriter, code int, data interface{}) {
	escapedJSON, err := json.MarshalIndent(&data, "", "  ")
	jsonData := bytes.Replace(escapedJSON, []byte("\\u0026"), []byte("&"), -1)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		w.Write(jsonData)
	} else {
		http.Error(w, err.Error(), 500)
	}
}

//ReturnError produces an error response with HTTP status code 500 if the given
//error is non-nil. Otherwise, nothing is done and false is returned.
func ReturnError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}

	http.Error(w, err.Error(), 500)
	return true
}

//RequireJSON will parse the request body into the given data structure, or
//write an error response if that fails.
func RequireJSON(w http.ResponseWriter, r *http.Request, data interface{}) bool {
	err := json.NewDecoder(r.Body).Decode(data)
	if err != nil {
		http.Error(w, "request body is not valid JSON: "+err.Error(), 400)
		return false
	}
	return true
}

//Path constructs a full URL for a given URL path below the /v1/ endpoint.
func (p *v1Provider) Path(elements ...string) string {
	parts := []string{
		strings.TrimSuffix( /*p.Driver.Cluster().Config.CatalogURL*/ "", "/"),
		"v1",
	}
	parts = append(parts, elements...)
	return strings.Join(parts, "/")
}
