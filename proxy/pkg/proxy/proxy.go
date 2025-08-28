// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"

	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/cache"
	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/informer"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/metricquery"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/util"
)

const (
	basePath               = "/api/metrics/v1/default"
	projectsAPIPath        = "/apis/project.openshift.io/v1/projects"
	userAPIPath            = "/apis/user.openshift.io/v1/users/~"
	apiSeriesPath          = "/api/v1/series"
	apiLabelNameValuesPath = "/api/v1/label/label_name/values"
	apiQueryPath           = "/api/v1/query"
	apiQueryRangePath      = "/api/v1/query_range"
)

// Proxy is a reverse proxy for the metrics server.
type Proxy struct {
	metricsServerURL       *url.URL
	apiServerHost          string
	proxy                  *httputil.ReverseProxy
	userProjectInfo        *cache.UserProjectInfo
	managedClusterInformer informer.ManagedClusterInformable
	accessReviewer         metricquery.AccessReviewer
}

// NewProxy creates a new Proxy.
func NewProxy(serverURL *url.URL, transport http.RoundTripper, apiserverHost string, upi *cache.UserProjectInfo, managedClusterInformer informer.ManagedClusterInformable, accessReviewer metricquery.AccessReviewer) (*Proxy, error) {
	p := &Proxy{
		metricsServerURL:       serverURL,
		apiServerHost:          apiserverHost,
		userProjectInfo:        upi,
		managedClusterInformer: managedClusterInformer,
		accessReviewer:         accessReviewer,
	}

	p.proxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			proxyRequest(req)
			req.URL.Scheme = serverURL.Scheme
			req.URL.Host = serverURL.Host
			req.Host = serverURL.Host
		},
		Transport: transport,
	}

	return p, nil
}

// ServeHTTP is used to init proxy handler.
func (p *Proxy) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	if err := p.preCheckRequest(req); err != nil {
		klog.Warningf("pre-check failed for user <%s>: %v", req.Header.Get("X-Forwarded-User"), err)
		res.Header().Set("Content-Type", "application/json")
		_, writeErr := res.Write(newEmptyMatrixHTTPBody())
		if writeErr != nil {
			klog.Errorf("failed to write response: %v", writeErr)
		}
		return
	}

	if ok := p.handleManagedClusterLabelQuery(res, req); ok {
		return
	}

	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = p.metricsServerURL.Host
	req.URL.Path = path.Join(basePath, req.URL.Path)
	modifier := &metricquery.Modifier{
		Req:            req,
		ReqURL:         config.GetConfigOrDie().Host + projectsAPIPath,
		AccessReviewer: p.accessReviewer,
		UPI:            p.userProjectInfo,
		MCI:            p.managedClusterInformer,
	}
	if err := modifier.Modify(); err != nil {
		klog.Errorf("failed to modify query: %v", err)
		http.Error(res, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	p.proxy.ServeHTTP(res, req)
}

func (p *Proxy) preCheckRequest(req *http.Request) error {
	token := req.Header.Get("X-Forwarded-Access-Token")
	if token == "" {
		token = req.Header.Get("Authorization")
		if token == "" {
			return errors.New("found unauthorized user")
		}
		token = strings.TrimPrefix(token, "Bearer ")
		req.Header.Set("X-Forwarded-Access-Token", token)
	}

	userName := req.Header.Get("X-Forwarded-User")
	if userName == "" {
		userAPIURL, err := url.JoinPath(p.apiServerHost, userAPIPath)
		if err != nil {
			return fmt.Errorf("failed to join the user api path with the apiserver host: %w", err)
		}
		userName, err = util.GetUserName(token, userAPIURL)
		if err != nil {
			return fmt.Errorf("failed to get user name: %w", err)
		}
		if userName == "" {
			return errors.New("failed to find user name")
		}
		req.Header.Set("X-Forwarded-User", userName)
	}

	if _, ok := p.userProjectInfo.GetUserProjectList(token); !ok {
		userProjectsURL, err := url.JoinPath(p.apiServerHost, projectsAPIPath)
		if err != nil {
			return fmt.Errorf("failed to join the user projects api path with the apiserver host: %w", err)
		}
		projectList, err := util.FetchUserProjectList(token, userProjectsURL)
		if err != nil {
			klog.Errorf("failed to fetch user project list: %v", err)
			// if we cannot fetch project list, we will just assume the user has no project access.
			projectList = []string{}
		}
		p.userProjectInfo.UpdateUserProject(userName, token, projectList)
	}

	if len(p.managedClusterInformer.GetAllManagedClusterNames()) == 0 {
		return errors.New("no project or cluster found")
	}

	return nil
}

// handleManagedClusterLabelQuery intercepts Grafana requests for the synthetic `acm_label_names` metric.
// This metric is generated within the proxy and does not exist upstream. The function directly returns
// a JSON response with the list of allowed label names from the informer's cache.
// It returns true if the request was handled, false otherwise.
func (p *Proxy) handleManagedClusterLabelQuery(res http.ResponseWriter, req *http.Request) bool {
	// This handler is only for the series and label values endpoints.
	isSeriesPath := strings.HasSuffix(req.URL.Path, apiSeriesPath)
	isLabelValuesPath := strings.HasSuffix(req.URL.Path, apiLabelNameValuesPath)
	if !isSeriesPath && !isLabelValuesPath {
		return false
	}

	isQuery, err := isACMLabelQuery(req)
	if err != nil {
		// An error here means we couldn't parse the request, so we can't handle it.
		// Let it fall through to the proxy to return a proper error.
		klog.Warningf("Could not determine if request is for ACM labels: %v", err)
		return false
	}

	if !isQuery {
		return false
	}

	// If we are here, it's a request for our synthetic metric. Handle it directly.
	managedLabelList := p.managedClusterInformer.GetManagedClusterLabelList()
	query, err := createQueryResponse(managedLabelList.RegexLabelList, proxyconfig.RBACProxyLabelMetricName, req.URL.Path)
	if err != nil {
		klog.Errorf("failed to create query response: %v", err)
		// Let the request fall through to the proxy to return a proper error.
		return false
	}

	res.Header().Set("Content-Type", "application/json")
	_, err = res.Write(query)
	if err != nil {
		klog.Errorf("failed to write query response: %v", err)
	}
	return true // We've handled the request.
}

// Structs for creating a JSON response for series queries.
type seriesData struct {
	Name      string `json:"__name__"`
	LabelName string `json:"label_name"`
}
type queryResponse struct {
	Status string `json:"status"`
	Data   any    `json:"data"`
}

func createQueryResponse(labels []string, metricName string, urlPath string) ([]byte, error) {
	var data any
	if strings.HasSuffix(urlPath, apiLabelNameValuesPath) {
		data = labels
	} else {
		series := make([]seriesData, len(labels))
		for i, label := range labels {
			series[i] = seriesData{
				Name:      metricName,
				LabelName: label,
			}
		}
		data = series
	}

	response := queryResponse{
		Status: "success",
		Data:   data,
	}

	return json.Marshal(response)
}

func newEmptyMatrixHTTPBody() []byte {
	return []byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`)
}

func proxyRequest(r *http.Request) {
	if r.Method == http.MethodGet {
		if strings.HasSuffix(r.URL.Path, apiQueryPath) ||
			strings.HasSuffix(r.URL.Path, apiQueryRangePath) ||
			strings.HasSuffix(r.URL.Path, apiSeriesPath) {
			r.Method = http.MethodPost
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Body = io.NopCloser(strings.NewReader(r.URL.RawQuery))
		}
	}
}

// isACMLabelQuery checks if an HTTP request is querying for the synthetic ACM label metric.
// It robustly parses the `match[]` parameters from either the URL query (for GET)
// or the request body (for POST) and checks for an exact match.
func isACMLabelQuery(req *http.Request) (bool, error) {
	var values url.Values
	var err error

	switch req.Method {
	case http.MethodGet:
		values = req.URL.Query()
	case http.MethodPost:
		// We need to read the body to check the 'match[]' param.
		// The body needs to be preserved so it can be read again by the proxy director.
		body, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			// Restore the body with an empty reader on error.
			req.Body = io.NopCloser(bytes.NewReader([]byte{}))
			return false, fmt.Errorf("failed to read request body: %w", readErr)
		}
		// Restore the body so it can be read again.
		req.Body = io.NopCloser(bytes.NewReader(body))

		values, err = url.ParseQuery(string(body))
		if err != nil {
			return false, fmt.Errorf("failed to parse post body: %w", err)
		}
	default:
		return false, nil
	}

	matchers := values["match[]"]
	for _, matcher := range matchers {
		if matcher == proxyconfig.RBACProxyLabelMetricName {
			return true, nil
		}
	}

	return false, nil
}
