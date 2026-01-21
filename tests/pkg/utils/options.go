// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

type TestOptionsContainer struct {
	Options TestOptions `json:"options"`
}

// Define options available for Tests to consume
type TestOptions struct {
	HubCluster      Cluster         `json:"hub"`
	ManagedClusters []Cluster       `json:"clusters"`
	ImageRegistry   Registry        `json:"imageRegistry"`
	KubeConfig      string          `json:"kubeconfig,omitempty"`
	Connection      CloudConnection `json:"cloudConnection"`
	Headless        string          `json:"headless,omitempty"`
	OwnerPrefix     string          `json:"ownerPrefix,omitempty"`
}

// Define the shape of clusters that may be added under management
type Cluster struct {
	Name             string          `json:"name,omitempty"`
	Namespace        string          `json:"namespace,omitempty"`
	Tags             map[string]bool `json:"tags,omitempty"`
	BaseDomain       string          `json:"baseDomain"`
	User             string          `json:"user,omitempty"`
	Password         string          `json:"password,omitempty"`
	KubeContext      string          `json:"kubecontext,omitempty"`
	ClusterServerURL string          `json:"clusterServerURL,omitempty"`
	GrafanaURL       string          `json:"grafanaURL,omitempty"`
	GrafanaHost      string          `json:"grafanaHost,omitempty"`
	KubeConfig       string          `json:"kubeconfig,omitempty"`
}

// Define the image registry
type Registry struct {
	// example: quay.io/stolostron
	Server   string `json:"server"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// CloudConnection struct for bits having to do with Connections
type CloudConnection struct {
	PullSecret    string  `json:"pullSecret"`
	SSHPrivateKey string  `json:"sshPrivatekey"`
	SSHPublicKey  string  `json:"sshPublickey"`
	Keys          APIKeys `json:"apiKeys"`
	OCPRelease    string  `json:"ocpRelease,omitempty"`
}

type APIKeys struct {
	AWS   AWSAPIKey   `json:"aws"`
	GCP   GCPAPIKey   `json:"gcp"`
	Azure AzureAPIKey `json:"azure"`
}

type AWSAPIKey struct {
	AWSAccessID     string `json:"awsAccessKeyID"`
	AWSAccessSecret string `json:"awsSecretAccessKeyID"`
	BaseDnsDomain   string `json:"baseDnsDomain"`
	Region          string `json:"region"`
}

type GCPAPIKey struct {
	ProjectID             string `json:"gcpProjectID"`
	ServiceAccountJsonKey string `json:"gcpServiceAccountJsonKey"`
	BaseDnsDomain         string `json:"baseDnsDomain"`
	Region                string `json:"region"`
}

type AzureAPIKey struct {
	BaseDnsDomain  string `json:"baseDnsDomain"`
	BaseDomainRGN  string `json:"azureBaseDomainRGN"`
	Region         string `json:"region"`
	SubscriptionID string `json:"subscriptionID"`
	TenantID       string `json:"tenantID"`
	ClientID       string `json:"clientID"`
	ClientSecret   string `json:"clientSecret"`
}
