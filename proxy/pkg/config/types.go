// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

// ClusterLabelList is the struct that contains the
// list of labels that are assigned to the managed clusters
type ClusterLabelList struct {
	LabelList []string `yaml:"labels"`
}
