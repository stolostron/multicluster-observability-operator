// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

// InstallConfig definition for install config structure from install-config.yaml
type InstallConfig struct {
	BaseDomain string     `json:"baseDomain,omitempty"`
	Networking Networking `json:"networking"`
	Metadata   Metadata   `json:"metadata"`
	Platform   Platform   `json:"platform"`
	PullSecret string     `json:"pullSecret,omitempty"`
	SSHKey     string     `json:"sshKey,omitempty"`
}

// Networking definition
type Networking struct {
	NetworkType string `json:"networkType"`
	MachineCIDR string `json:"machineCIDR"`
}

// Platform definition
type Platform struct {
	Baremetal Baremetal `json:"baremetal"`
}

// Baremetal specs for target baremetal provisioning
type Baremetal struct {
	ExternalBridge               string `json:"externalBridge,omitempty"`
	ProvisioningBridge           string `json:"provisioningBridge,omitempty"`
	LibvirtURI                   string `json:"libvirtURI,omitempty"`
	ProvisioningNetworkInterface string `json:"provisioningNetworkInterface,omitempty"`
	ProvisioningNetworkCIDR      string `json:"provisioningNetworkCIDR,omitempty"`
	APIVIP                       string `json:"apiVIP,omitempty"`
	DNSVIP                       string `json:"dnsVIP,omitempty"`
	IngressVIP                   string `json:"ingressVIP,omitempty"`
	Hosts                        []Host `json:"hosts,omitempty"`
	SSHKnownHosts                string `json:"sshKnownHosts,omitempty"`
}

// Host is an array of baremetal assets
type Host struct {
	Name            string `json:"name"`
	Role            string `json:"role"`
	Bmc             Bmc    `json:"bmc"`
	BootMACAddress  string `json:"bootMACAddress"`
	HardwareProfile string `json:"hardwareProfile"`
}

// Bmc definition
type Bmc struct {
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
}
