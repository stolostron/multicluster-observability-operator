// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

func GetGrafanaURL(opt TestOptions) string {
	grafanaConsoleURL := "https://grafana-open-cluster-management-observability.apps." + opt.HubCluster.BaseDomain
	if opt.HubCluster.GrafanaURL != "" {
		grafanaConsoleURL = opt.HubCluster.GrafanaURL
	} else {
		opt.HubCluster.GrafanaHost = "grafana-open-cluster-management-observability.apps." + opt.HubCluster.BaseDomain
	}
	return grafanaConsoleURL
}
