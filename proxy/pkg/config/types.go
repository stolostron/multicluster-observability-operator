// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

// ManagedClusterLabelList is the struct that contains the
// list of labels that are assigned to the managed clusters
type ManagedClusterLabelList struct {
	BlackList      []string `yaml:"blacklist_labels,omitempty"`
	LabelList      []string `yaml:"labels"`
	RegexLabelList []string `yaml:"-"`
}
