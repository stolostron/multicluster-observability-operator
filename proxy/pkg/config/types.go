// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

// ManagedClusterLabelList is the struct that contains the
// list of labels that are assigned to the managed clusters
type ManagedClusterLabelList struct {
	LabelList []string `yaml:"labels"`
	BlackList []string `yaml:"labels"`
}
