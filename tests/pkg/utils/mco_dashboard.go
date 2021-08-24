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

func ContainDashboard(opt TestOptions, title string) (error, bool) {
	grafanaConsoleURL := GetGrafanaURL(opt)
	path := "/api/search?"
	queryParams := url.PathEscape(fmt.Sprintf("query=%s", title))
	req, err := http.NewRequest(
		"GET",
		grafanaConsoleURL+path+queryParams,
		nil)
	if err != nil {
		return err, false
	}

	client := &http.Client{}
	if os.Getenv("IS_KIND_ENV") != "true" {
		tr := &http.Transport{
			/* #nosec */
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		client = &http.Client{Transport: tr}
		token, err := FetchBearerToken(opt)
		if err != nil {
			return err, false
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Host = opt.HubCluster.GrafanaHost
	}

	resp, err := client.Do(req)
	if err != nil {
		return err, false
	}

	if resp.StatusCode != http.StatusOK {
		klog.Errorf("resp: %+v\n", resp)
		klog.Errorf("err: %+v\n", err)
		return fmt.Errorf("failed to access grafana api"), false
	}

	result, err := ioutil.ReadAll(resp.Body)
	klog.V(1).Infof("result: %s\n", result)
	if err != nil {
		return err, false
	}

	if !strings.Contains(string(result), fmt.Sprintf(`"title":"%s"`, title)) {
		return fmt.Errorf("failed to find the dashboard"), false
	} else {
		return nil, true
	}
}
