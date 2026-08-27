// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// ValidateClusterServerURL ensures that a clusterServerURL contains a valid scheme, host, and optional port.
func ValidateClusterServerURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("clusterServerURL cannot be empty")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("failed to parse clusterServerURL %q: %w", rawURL, err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid clusterServerURL %q: scheme must be 'http' or 'https', got %q", rawURL, u.Scheme)
	}

	if u.Hostname() == "" {
		return fmt.Errorf("invalid clusterServerURL %q: missing host/domain", rawURL)
	}

	if portStr := u.Port(); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("invalid clusterServerURL %q: port %q is out of range (1-65535)", rawURL, portStr)
		}
	}

	return nil
}

type TestOptionsContainer struct {
	Options TestOptions `yaml:"options"`
}

// Define options available for Tests to consume
type TestOptions struct {
	HubCluster      Cluster         `yaml:"hub"`
	ManagedClusters []Cluster       `yaml:"clusters"`
	ImageRegistry   Registry        `yaml:"imageRegistry,omitempty"`
	KubeConfig      string          `yaml:"kubeconfig,omitempty"`
	Connection      CloudConnection `yaml:"cloudConnection,omitempty"`
	Headless        string          `yaml:"headless,omitempty"`
	OwnerPrefix     string          `yaml:"ownerPrefix,omitempty"`
}

// Define the shape of clusters that may be added under management
type Cluster struct {
	Name             string          `yaml:"name,omitempty"`
	Namespace        string          `yaml:"namespace,omitempty"`
	Tags             map[string]bool `yaml:"tags,omitempty"`
	BaseDomain       string          `yaml:"baseDomain"`
	User             string          `yaml:"user,omitempty"`
	Password         string          `yaml:"password,omitempty"`
	KubeContext      string          `yaml:"kubecontext,omitempty"`
	ClusterServerURL string          `yaml:"clusterServerURL,omitempty"`
	GrafanaURL       string          `yaml:"grafanaURL,omitempty"`
	GrafanaHost      string          `yaml:"grafanaHost,omitempty"`
	KubeConfig       string          `yaml:"kubeconfig,omitempty"`
}

// Define the image registry
type Registry struct {
	// example: quay.io/stolostron
	Server   string `yaml:"server"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// CloudConnection struct for bits having to do with Connections
type CloudConnection struct {
	PullSecret    string  `yaml:"pullSecret"`
	SSHPrivateKey string  `yaml:"sshPrivatekey"`
	SSHPublicKey  string  `yaml:"sshPublickey"`
	Keys          APIKeys `yaml:"apiKeys,omitempty"`
	OCPRelease    string  `yaml:"ocpRelease,omitempty"`
}

type APIKeys struct {
	AWS   AWSAPIKey   `yaml:"aws,omitempty"`
	GCP   GCPAPIKey   `yaml:"gcp,omitempty"`
	Azure AzureAPIKey `yaml:"azure,omitempty"`
}

type AWSAPIKey struct {
	AWSAccessID     string `yaml:"awsAccessKeyID"`
	AWSAccessSecret string `yaml:"awsSecretAccessKeyID"`
	BaseDnsDomain   string `yaml:"baseDnsDomain"`
	Region          string `yaml:"region"`
}

type GCPAPIKey struct {
	ProjectID             string `yaml:"gcpProjectID"`
	ServiceAccountJsonKey string `yaml:"gcpServiceAccountJsonKey"`
	BaseDnsDomain         string `yaml:"baseDnsDomain"`
	Region                string `yaml:"region"`
}

type AzureAPIKey struct {
	BaseDnsDomain  string `yaml:"baseDnsDomain"`
	BaseDomainRGN  string `yaml:"azureBaseDomainRGN"`
	Region         string `yaml:"region"`
	SubscriptionID string `yaml:"subscriptionID"`
	TenantID       string `yaml:"tenantID"`
	ClientID       string `yaml:"clientID"`
	ClientSecret   string `yaml:"clientSecret"`
}
