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
		return nil, fmt.Errorf("failed to new http request: %w", err)
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

func FetchUserProjectList(token string, url string) ([]string, error) {
	return FetchUserProjectListWithClient(http.DefaultClient, token, url)
}

func FetchUserProjectListWithClient(client *http.Client, token string, url string) ([]string, error) {
	resp, err := sendHTTPRequestWithClient(client, url, "GET", token)
	if err != nil {
		return nil, fmt.Errorf("failed to send http request: %w", err)
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
		return nil, fmt.Errorf("failed to decode response json body: %w", err)
	}

	projectList := make([]string, len(projects.Items))
	for idx, p := range projects.Items {
		projectList[idx] = p.Name
	}

	return projectList, nil
}

func GetUserName(token string, url string) (string, error) {
	return GetUserNameWithClient(http.DefaultClient, token, url)
}

func GetUserNameWithClient(client *http.Client, token string, url string) (string, error) {
	resp, err := sendHTTPRequestWithClient(client, url, "GET", token)
	if err != nil {
		return "", fmt.Errorf("failed to send http request: %w", err)
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
		return "", fmt.Errorf("failed to decode response json body: %w", err)
	}

	return user.Name, nil
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

	if err = f.Close(); err != nil {
		klog.Errorf("failed to close probe file: %v", err)
	}
}

