// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"fmt"
	"os"

	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FetchUserProjectList(ctx context.Context, c client.Client) ([]string, error) {
	projects := &projectv1.ProjectList{}
	err := c.List(ctx, projects)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	projectList := make([]string, len(projects.Items))
	for i, p := range projects.Items {
		projectList[i] = p.Name
	}
	return projectList, nil
}

func GetUserName(ctx context.Context, c client.Client) (string, error) {
	user := &userv1.User{}
	// The "~" is a special OpenShift API shortcut to get the user associated with the request token.
	err := c.Get(ctx, client.ObjectKey{Name: "~"}, user)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
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
