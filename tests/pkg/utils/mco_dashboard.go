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

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: tr}
	if os.Getenv("IS_CANARY_ENV") == "true" {
		token, err := FetchBearerToken(opt)
		if err != nil {
			return err, false
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	} else {
		req.Header.Set("X-Forwarded-User", "WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000")
	}
	req.Host = opt.HubCluster.GrafanaHost

	resp, err := client.Do(req)
	if err != nil {
		return err, false
	}

	if resp.StatusCode != http.StatusOK {
		klog.Errorf("resp.StatusCode: %v\n", resp.StatusCode)
		return fmt.Errorf("Failed to access grafana api"), false
	}

	result, err := ioutil.ReadAll(resp.Body)
	klog.V(1).Infof("result: %s\n", result)
	if err != nil {
		return err, false
	}

	if !strings.Contains(string(result), fmt.Sprintf(`"title":"%s"`, title)) {
		return fmt.Errorf("Failed to find the dashboard"), false
	} else {
		return nil, true
	}
}
