// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	"k8s.io/klog"
)

func sendHTTPRequestWithClient(client *http.Client, url string, verb string, token string) (*http.Response, error) {
	req, err := http.NewRequest(verb, url, nil)
	if err != nil {
		klog.Errorf("failed to new http request: %v", err)
		return nil, err
	}

	if len(token) == 0 {
		transport := &http.Transport{}
		defaultClient := &http.Client{Transport: transport}
		return defaultClient.Do(req)
	}

	if !strings.HasPrefix(token, "Bearer ") {
		token = "Bearer " + token
	}
	req.Header.Set("Authorization", token)
	return client.Do(req)
}

func FetchUserProjectList(token string, url string) []string {
	return FetchUserProjectListWithClient(http.DefaultClient, token, url)
}

func FetchUserProjectListWithClient(client *http.Client, token string, url string) []string {
	resp, err := sendHTTPRequestWithClient(client, url, "GET", token)
	if err != nil {
		klog.Errorf("failed to send http request: %v", err)
		/*
		   This is adhoc step to make sure that if this error happens,
		   we can automatically restart the POD using liveness probe which checks for this file.
		   Once the real cause is determined and fixed, we will remove this.
		*/
		writeError(fmt.Sprintf("failed to send http request: %v", err))
		return []string{}
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			klog.Errorf("failed to close response body: %v", err)
		}
	}()

	var projects projectv1.ProjectList
	err = json.NewDecoder(resp.Body).Decode(&projects)
	if err != nil {
		klog.Errorf("failed to decode response json body: %v", err)
		return []string{}
	}

	projectList := make([]string, len(projects.Items))
	for idx, p := range projects.Items {
		projectList[idx] = p.Name
	}

	return projectList
}

func GetUserName(token string, url string) string {
	return GetUserNameWithClient(http.DefaultClient, token, url)
}

func GetUserNameWithClient(client *http.Client, token string, url string) string {
	resp, err := sendHTTPRequestWithClient(client, url, "GET", token)
	if err != nil {
		klog.Errorf("failed to send http request: %v", err)
		writeError(fmt.Sprintf("failed to send http request: %v", err))
		return ""
	}

	user := userv1.User{}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			klog.Errorf("failed to close response body: %v", err)
		}
	}()

	err = json.NewDecoder(resp.Body).Decode(&user)
	if err != nil {
		klog.Errorf("failed to decode response json body: %v", err)
		return ""
	}

	return user.Name
}

var healthCheckFilePath = "/tmp/health"

func writeError(msg string) {
	f, err := os.OpenFile(healthCheckFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		klog.Errorf("failed to create file for probe: %v", err)
	}

	_, err = f.Write([]byte(msg))
	if err != nil {
		klog.Errorf("failed to write error message to probe file: %v", err)
	}

	_ = f.Close()
}