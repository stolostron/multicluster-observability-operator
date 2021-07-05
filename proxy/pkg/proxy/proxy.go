// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package proxy

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"

	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/open-cluster-management/multicluster-observability-operator/proxy/pkg/util"
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

// HandleRequestAndRedirect is used to init proxy handler
func HandleRequestAndRedirect(res http.ResponseWriter, req *http.Request) {
	if preCheckRequest(req) != nil {
		_, err := res.Write(newEmptyMatrixHTTPBody())
		if err != nil {
			klog.Errorf("failed to write response: %v", err)
		}
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
	util.ModifyMetricsQueryParams(req, config.GetConfigOrDie().Host+projectsAPIPath)
	proxy.ServeHTTP(res, req)
}

func errorHandle(rw http.ResponseWriter, req *http.Request, err error) {
	token := req.Header.Get("X-Forwarded-Access-Token")
	if token == "" {
		rw.WriteHeader(http.StatusUnauthorized)
	}
}

func preCheckRequest(req *http.Request) error {
	token := req.Header.Get("X-Forwarded-Access-Token")
	if token == "" {
		token = req.Header.Get("Authorization")
		if token == "" {
			return errors.New("found unauthorized user")
		} else {
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

	projectList, ok := util.GetUserProjectList(token)
	if !ok {
		projectList = util.FetchUserProjectList(token, config.GetConfigOrDie().Host+projectsAPIPath)
		up := util.NewUserProject(userName, token, projectList)
		util.UpdateUserProject(up)
	}

	if len(projectList) == 0 || len(util.GetAllManagedClusterNames()) == 0 {
		return errors.New("no project or cluster found")
	}

	return nil
}

func newEmptyMatrixHTTPBody() []byte {
	var bodyBuff bytes.Buffer
	gz := gzip.NewWriter(&bodyBuff)
	if _, err := gz.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`)); err != nil {
		klog.Errorf("failed to write body: %v", err)
	}

	if err := gz.Close(); err != nil {
		klog.Errorf("failed to close gzip writer: %v", err)
	}

	var gzipBuff bytes.Buffer
	err := gzipWrite(&gzipBuff, bodyBuff.Bytes())
	if err != nil {
		klog.Errorf("failed to write with gizp: %v", err)
	}

	return gzipBuff.Bytes()
}

func gzipWrite(w io.Writer, data []byte) error {
	gw, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
	if err != nil {
		return err
	}
	defer gw.Close()
	_, err = gw.Write(data)
	return err
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
			r.Body = ioutil.NopCloser(strings.NewReader(r.URL.RawQuery))
		}
	}
}
