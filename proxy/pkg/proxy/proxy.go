// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"

	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/proxy/pkg/util"
)

const (
	basePath        = "/api/metrics/v1/default"
	projectsAPIPath = "/apis/project.openshift.io/v1/projects"
	userAPIPath     = "/apis/user.openshift.io/v1/users/~"
)

var (
	serverScheme = ""
	serverHost   = ""
)

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

// HandleRequestAndRedirect is used to init proxy handler.
func HandleRequestAndRedirect(res http.ResponseWriter, req *http.Request) {
	if preCheckRequest(req) != nil {
		_, err := res.Write(newEmptyMatrixHTTPBody())
		if err != nil {
			klog.Errorf("failed to write response: %v", err)
		}
		return
	}

	if ok := shouldModifyAPISeriesResponse(res, req); ok {
		return
	}

	serverURL, err := url.Parse(os.Getenv("METRICS_SERVER"))
	if err != nil {
		klog.Errorf("failed to parse url: %v", err)
	}
	serverHost = serverURL.Host
	serverScheme = serverURL.Scheme

	tlsTransport, err := getTLSTransport()
	if err != nil {
		klog.Fatalf("failed to create tls transport: %v", err)
	}

	// create the reverse proxy
	proxy := httputil.ReverseProxy{
		Director:  proxyRequest,
		Transport: tlsTransport,
	}

	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = serverURL.Host
	req.URL.Path = path.Join(basePath, req.URL.Path)
	util.ModifyMetricsQueryParams(req, config.GetConfigOrDie().Host+projectsAPIPath, util.GetAccessReviewer())
	proxy.ServeHTTP(res, req)
}

func preCheckRequest(req *http.Request) error {
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
		userName = util.GetUserName(token, config.GetConfigOrDie().Host+userAPIPath)
		if userName == "" {
			return errors.New("failed to found user name")
		} else {
			req.Header.Set("X-Forwarded-User", userName)
		}
	}

	_, ok := util.GetUserProjectList(token)
	if !ok {
		projectList := util.FetchUserProjectList(token, config.GetConfigOrDie().Host+projectsAPIPath)
		up := util.NewUserProject(userName, token, projectList)
		util.UpdateUserProject(up)
	}

	if len(util.GetAllManagedClusterNames()) == 0 {
		return errors.New("no project or cluster found")
	}

	return nil
}

func newEmptyMatrixHTTPBody() []byte {
	return []byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`)
}

func proxyRequest(r *http.Request) {
	r.URL.Scheme = serverScheme
	r.URL.Host = serverHost
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
