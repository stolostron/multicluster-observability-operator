// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

// ClusterDeploy defines the data passed to Hive
type ClusterDeploy struct {
	Kind       string  `json:"kind"`
	APIVersion string  `json:"apiVersion"`
	Items      []Items `json:"items"`
}

// Items defines the list of items in the cluster deploy yaml
type Items struct {
	Kind       string     `json:"kind"`
	Metadata   Metadata   `json:"metadata"`
	StringData StringData `json:"stringData,omitempty"`
	Spec       Spec       `json:"spec,omitempty"`
}

// Metadata defines the name
type Metadata struct {
	Name string `json:"name,omitempty"`
}

// StringData defiines the ssh values
type StringData struct {
	Dockerconfigjson string `json:".dockerconfigjson,omitempty"`
	SSHPrivateKey    string `json:"ssh-privatekey,omitempty"`
}

// Spec defines the kube specifications
type Spec struct {
	BaseDomain   string       `json:"baseDomain,omitempty"`
	ClusterName  string       `json:"clusterName,omitempty"`
	Provisioning Provisioning `json:"provisioning,omitempty"`
}

// Provisioning defines the data related to cluster creation
type Provisioning struct {
	ReleaseImage  string   `json:"releaseImage,omitempty"`
	SSHKnownHosts []string `json:"sshKnownHosts,omitempty"`
}
