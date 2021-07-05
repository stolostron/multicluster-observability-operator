// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"k8s.io/klog"
)

func ContainManagedClusterMetric(opt TestOptions, query string, matchedLabels []string) (error, bool) {
	grafanaConsoleURL := GetGrafanaURL(opt)
	path := "/api/datasources/proxy/1/api/v1/query?"
	// TODO(morvencao): remove this after accessing metrics from grafana url with bearer token is supported
	if os.Getenv("IS_CANARY_ENV") != "true" && os.Getenv("THANOS_QUERY_FRONTEND_URL") != "" {
		grafanaConsoleURL = os.Getenv("THANOS_QUERY_FRONTEND_URL")
		path = "/api/v1/query?"
	}
	queryParams := url.PathEscape(fmt.Sprintf("query=%s", query))
	klog.V(5).Infof("request url is: %s\n", grafanaConsoleURL+path+queryParams)
	req, err := http.NewRequest(
		"GET",
		grafanaConsoleURL+path+queryParams,
		nil)
	if err != nil {
		return err, false
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	token, err := FetchBearerToken(opt)
	if err != nil {
		return err, false
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Host = opt.HubCluster.GrafanaHost

	resp, err := client.Do(req)
	if err != nil {
		return err, false
	}

	if resp.StatusCode != http.StatusOK {
		klog.Errorf("resp.StatusCode: %v\n", resp.StatusCode)
		return fmt.Errorf("Failed to access managed cluster metrics via grafana console"), false
	}

	metricResult, err := ioutil.ReadAll(resp.Body)
	klog.V(5).Infof("metricResult: %s\n", metricResult)
	if err != nil {
		return err, false
	}

	if !strings.Contains(string(metricResult), `"status":"success"`) {
		return fmt.Errorf("Failed to find valid status from response"), false
	}

	if strings.Contains(string(metricResult), `"result":[]`) {
		return fmt.Errorf("Failed to find metric name from response"), false
	}

	contained := true
	for _, label := range matchedLabels {
		if !strings.Contains(string(metricResult), label) {
			contained = false
			break
		}
	}
	if !contained {
		return fmt.Errorf("Failed to find metric name from response"), false
	}

	return nil, true
}
