// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

func GetGrafanaURL(opt TestOptions) string {
	grafanaConsoleURL := "https://multicloud-console.apps." + opt.HubCluster.BaseDomain + "/grafana/"
	if opt.HubCluster.GrafanaURL != "" {
		grafanaConsoleURL = opt.HubCluster.GrafanaURL
	} else {
		opt.HubCluster.GrafanaHost = "multicloud-console.apps." + opt.HubCluster.BaseDomain
	}
	return grafanaConsoleURL
}
