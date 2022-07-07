// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

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
