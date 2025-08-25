// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package proxy

import (
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

	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/informer"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/util"
)

const (
	basePath        = "/api/metrics/v1/default"
	projectsAPIPath = "/apis/project.openshift.io/v1/projects"
	userAPIPath     = "/apis/user.openshift.io/v1/users/~"
)

// Proxy is a reverse proxy for the metrics server.
type Proxy struct {
	metricsServerURL *url.URL
	apiServerHost    string
	proxy            *httputil.ReverseProxy
	userProjectInfo  *util.UserProjectInfo
}

// NewProxy creates a new Proxy.
func NewProxy(serverURL *url.URL, transport http.RoundTripper, apiserverHost string, upi *util.UserProjectInfo) (*Proxy, error) {
	p := &Proxy{
		metricsServerURL: serverURL,
		proxy: &httputil.ReverseProxy{
			Director:  proxyRequest,
			Transport: transport,
		},
		apiServerHost:   apiserverHost,
		userProjectInfo: upi,
	}

	return p, nil
}

func requestContainsRBACProxyLabeMetricName(req *http.Request) bool {
	if req.Method == "POST" {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			klog.Errorf("failed to read body: %v", err)
		}
		req.Body = io.NopCloser(strings.NewReader(string(body)))
		req.ContentLength = int64(len([]rune(string(body))))
		return strings.Contains(string(body), proxyconfig.GetRBACProxyLabelMetricName())
	} else if req.Method == "GET" {
		queryParams := req.URL.Query()
		return strings.Contains(queryParams.Get("match[]"), proxyconfig.GetRBACProxyLabelMetricName())
	}
	return false
}

func shouldModifyAPISeriesResponse(res http.ResponseWriter, req *http.Request) bool {
	// Different Grafana versions uses different calls, we handle:
	// GET/POST requests for series and label_name
	if strings.HasSuffix(req.URL.Path, "/api/v1/series") ||
		strings.HasSuffix(req.URL.Path, "/api/v1/label/label_name/values") {
		if requestContainsRBACProxyLabeMetricName(req) {
			managedLabelList := proxyconfig.GetManagedClusterLabelList()

			query := createQueryResponse(managedLabelList.RegexLabelList, proxyconfig.GetRBACProxyLabelMetricName(), req.URL.Path)
			_, err := res.Write([]byte(query))
			if err == nil {
				return true
			} else {
				klog.Errorf("failed to write query: %v", err)
			}
		}

	}

	return false
}

func createQueryResponse(labels []string, metricName string, urlPath string) string {
	query := `{"status":"success","data":[`
	for index, label := range labels {
		if strings.HasSuffix(urlPath, "/api/v1/label/label_name/values") {
			query += `"` + label + `"`
		} else {
			// series
			query += `{"__name__":"` + metricName + `","label_name":"` + label + `"}`
		}
		if index != len(labels)-1 {
			query += ","
		}
	}
	query += `]}`
	return query
}

// ServeHTTP is used to init proxy handler.
func (p *Proxy) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	if p.preCheckRequest(req) != nil {
		_, err := res.Write(newEmptyMatrixHTTPBody())
		if err != nil {
			klog.Errorf("failed to write response: %v", err)
		}
		return
	}

	if ok := shouldModifyAPISeriesResponse(res, req); ok {
		return
	}

	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = p.metricsServerURL.Host
	req.URL.Path = path.Join(basePath, req.URL.Path)
	util.ModifyMetricsQueryParams(req, config.GetConfigOrDie().Host+projectsAPIPath, util.GetAccessReviewer(), p.userProjectInfo)
	p.proxy.ServeHTTP(res, req)
}

func (p *Proxy) preCheckRequest(req *http.Request) error {
	token := req.Header.Get("X-Forwarded-Access-Token")
	if token == "" {
		token = req.Header.Get("Authorization")
		if token == "" {
			return errors.New("found unauthorized user")
		} else {
			// Remove Bearer from token if present
			token = strings.TrimPrefix(token, "Bearer ")
			req.Header.Set("X-Forwarded-Access-Token", token)
		}
	}

	userName := req.Header.Get("X-Forwarded-User")
	if userName == "" {
		userAPIURL, err := url.JoinPath(p.apiServerHost, userAPIPath)
		if err != nil {
			return fmt.Errorf("failed to join the user api path with the apiserver host: %w", err)
		}
		userName = util.GetUserName(token, userAPIURL)
		if userName == "" {
			return errors.New("failed to find user name")
		} else {
			req.Header.Set("X-Forwarded-User", userName)
		}
	}

	_, ok := p.userProjectInfo.GetUserProjectList(token)
	if !ok {
		userProjectsURL, err := url.JoinPath(p.apiServerHost, projectsAPIPath)
		if err != nil {
			return fmt.Errorf("failed to join the user projects api path with the apiserver host: %w", err)

		}
		projectList := util.FetchUserProjectList(token, userProjectsURL)
		p.userProjectInfo.UpdateUserProject(userName, token, projectList)
	}

	if len(informer.GetAllManagedClusterNames()) == 0 {
		return errors.New("no project or cluster found")
	}

	return nil
}

func newEmptyMatrixHTTPBody() []byte {
	return []byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`)
}

func proxyRequest(r *http.Request) {
	if r.Method == http.MethodGet {
		if strings.HasSuffix(r.URL.Path, "/api/v1/query") ||
			strings.HasSuffix(r.URL.Path, "/api/v1/query_range") ||
			strings.HasSuffix(r.URL.Path, "/api/v1/series") {
			r.Method = http.MethodPost
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Body = io.NopCloser(strings.NewReader(r.URL.RawQuery))
		}
	}
}
