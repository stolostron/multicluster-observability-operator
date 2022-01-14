// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

// InstallConfig definition for install config structure from install-config.yaml
type InstallConfig struct {
	BaseDomain string     `yaml:"baseDomain,omitempty"`
	Networking Networking `yaml:"networking,omitempty"`
	Metadata   Metadata   `yaml:"metadata"`
	Platform   Platform   `yaml:"platform,omitempty"`
	PullSecret string     `yaml:"pullSecret,omitempty"`
	SSHKey     string     `yaml:"sshKey,omitempty"`
}

// Networking definition
type Networking struct {
	NetworkType string `yaml:"networkType"`
	MachineCIDR string `yaml:"machineCIDR"`
}

// Platform definition
type Platform struct {
	Baremetal Baremetal `yaml:"baremetal,omitempty"`
}

// Baremetal specs for target baremetal provisioning
type Baremetal struct {
	ExternalBridge               string `yaml:"externalBridge,omitempty"`
	ProvisioningBridge           string `yaml:"provisioningBridge,omitempty"`
	LibvirtURI                   string `yaml:"libvirtURI,omitempty"`
	ProvisioningNetworkInterface string `yaml:"provisioningNetworkInterface,omitempty"`
	ProvisioningNetworkCIDR      string `yaml:"provisioningNetworkCIDR,omitempty"`
	APIVIP                       string `yaml:"apiVIP,omitempty"`
	DNSVIP                       string `yaml:"dnsVIP,omitempty"`
	IngressVIP                   string `yaml:"ingressVIP,omitempty"`
	Hosts                        []Host `yaml:"hosts,omitempty"`
	SSHKnownHosts                string `yaml:"sshKnownHosts,omitempty"`
}

// Host is an array of baremetal assets
type Host struct {
	Name            string `yaml:"name"`
	Role            string `yaml:"role"`
	Bmc             Bmc    `yaml:"bmc"`
	BootMACAddress  string `yaml:"bootMACAddress"`
	HardwareProfile string `yaml:"hardwareProfile"`
}

// Bmc definition
type Bmc struct {
	Address  string `yaml:"address"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}
