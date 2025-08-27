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

	if ok := p.shouldModifyAPISeriesResponse(res, req); ok {
		return
	}

	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = p.metricsServerURL.Host
	req.URL.Path = path.Join(basePath, req.URL.Path)
	(&metricquery.Modifier{
		Req:            req,
		ReqURL:         config.GetConfigOrDie().Host + projectsAPIPath,
		AccessReviewer: p.accessReviewer,
		UPI:            p.userProjectInfo,
		MCI:            p.managedClusterInformer,
	}).Modify()
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
		userName = util.GetUserName(token, userAPIURL)
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
		projectList := util.FetchUserProjectList(token, userProjectsURL)
		p.userProjectInfo.UpdateUserProject(userName, token, projectList)
	}

	if len(p.managedClusterInformer.GetAllManagedClusterNames()) == 0 {
		return errors.New("no project or cluster found")
	}

	return nil
}

func (p *Proxy) shouldModifyAPISeriesResponse(res http.ResponseWriter, req *http.Request) bool {
	// Different Grafana versions use different calls, we handle:
	// GET/POST requests for series and label_name
	if strings.HasSuffix(req.URL.Path, apiSeriesPath) ||
		strings.HasSuffix(req.URL.Path, apiLabelNameValuesPath) {
		if requestContainsRBACProxyLabelMetricName(req) {
			managedLabelList := p.managedClusterInformer.GetManagedClusterLabelList()

			query, err := createQueryResponse(managedLabelList.RegexLabelList, proxyconfig.GetRBACProxyLabelMetricName(), req.URL.Path)
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
	}
	return false
}

// Structs for creating a JSON response for series queries.
type seriesData struct {
	Name      string `json:"__name__"`
	LabelName string `json:"label_name"`
}
type queryResponse struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data"`
}

func createQueryResponse(labels []string, metricName string, urlPath string) ([]byte, error) {
	var data interface{}
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

func requestContainsRBACProxyLabelMetricName(req *http.Request) bool {
	switch req.Method {
	case http.MethodPost:
		body, err := io.ReadAll(req.Body)
		if err != nil {
			klog.Errorf("failed to read body: %v", err)
			req.Body = io.NopCloser(bytes.NewReader(body))
			return false
		}
		// Replace the body so it can be read again downstream.
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		return strings.Contains(string(body), proxyconfig.GetRBACProxyLabelMetricName())
	case http.MethodGet:
		return strings.Contains(req.URL.Query().Get("match[]"), proxyconfig.GetRBACProxyLabelMetricName())
	default:
		return false
	}
}
