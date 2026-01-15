// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"crypto/sha256"
	"fmt"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"open-cluster-management.io/addon-framework/pkg/agent"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	addonName = "observability-controller" // #nosec G101 -- Not a hardcoded credential.
	agentName = "observability"
)

type ObservabilityAgent struct {
	client client.Client
}

func (o *ObservabilityAgent) Manifests(
	cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn,
) ([]runtime.Object, error) {
	return nil, nil
}

func (o *ObservabilityAgent) GetAgentAddonOptions() agent.AgentAddonOptions {
	signAdaptor := func(csr *certificatesv1.CertificateSigningRequest) []byte {
		res, err := Sign(o.client, csr)
		if err != nil {
			log.Error(err, "failed to sign")
		}
		return res
	}
	return agent.AgentAddonOptions{
		AddonName: addonName,
		Registration: &agent.RegistrationOption{
			CSRConfigurations: observabilitySignerConfigurations(o.client),
			CSRApproveCheck:   approve,
			PermissionConfig: func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
				return nil
			},
			CSRSign: signAdaptor,
		},
	}
}

func observabilitySignerConfigurations(client client.Client) func(cluster *clusterv1.ManagedCluster) []addonapiv1alpha1.RegistrationConfig {
	return func(cluster *clusterv1.ManagedCluster) []addonapiv1alpha1.RegistrationConfig {
		observabilityConfig := addonapiv1alpha1.RegistrationConfig{
			SignerName: "open-cluster-management.io/observability-signer",
			Subject: addonapiv1alpha1.Subject{
				User:              "managed-cluster-observability",
				OrganizationUnits: []string{"acm"},
			},
		}
		kubeClientSignerConfigurations := agent.KubeClientSignerConfigurations(addonName, agentName)
		registrationConfigs := append(kubeClientSignerConfigurations(cluster), observabilityConfig)

		// Get CA certificate for hash stamping
		_, _, caCertBytes, err := getCA(client, true)
		if err != nil {
			log.Error(err, "Failed to get CA certificate for hash stamping")
		} else if len(caCertBytes) > 0 { // Only stamp if we actually got a CA cert
			caHashStamp := fmt.Sprintf("ca-hash-%x", sha256.Sum256(caCertBytes))
			for i := range registrationConfigs {
				registrationConfigs[i].Subject.OrganizationUnits = append(registrationConfigs[i].Subject.OrganizationUnits, caHashStamp)
			}
		}

		return registrationConfigs
	}
}
