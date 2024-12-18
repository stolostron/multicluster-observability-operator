// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

// ClusterDeploy defines the data passed to Hive
type ClusterDeploy struct {
	Kind       string  `yaml:"kind"`
	APIVersion string  `yaml:"apiVersion"`
	Items      []Items `yaml:"items"`
}

// Items defines the list of items in the cluster deploy yaml
type Items struct {
	Kind       string     `yaml:"kind"`
	Metadata   Metadata   `yaml:"metadata"`
	StringData StringData `yaml:"stringData,omitempty"`
	Spec       Spec       `yaml:"spec,omitempty"`
}

// Metadata defines the name
type Metadata struct {
	Name string `yaml:"name,omitempty"`
}

// StringData defiines the ssh values
type StringData struct {
	Dockerconfigjson string `yaml:".dockerconfigjson,omitempty"`
	SSHPrivateKey    string `yaml:"ssh-privatekey,omitempty"`
}

// Spec defines the kube specifications
type Spec struct {
	BaseDomain   string       `yaml:"baseDomain,omitempty"`
	ClusterName  string       `yaml:"clusterName,omitempty"`
	Provisioning Provisioning `yaml:"provisioning,omitempty"`
}

// Provisioning defines the data related to cluster creation
type Provisioning struct {
	ReleaseImage  string   `yaml:"releaseImage,omitempty"`
	SSHKnownHosts []string `yaml:"sshKnownHosts,omitempty"`
}
