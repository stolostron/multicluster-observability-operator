// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"crypto/sha256"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"open-cluster-management.io/addon-framework/pkg/agent"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
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
	return agent.AgentAddonOptions{
		AddonName: addonName,
		Registration: &agent.RegistrationOption{
			CSRConfigurations: observabilitySignerConfigurations(o.client),
			CSRApproveCheck:   approve,
			PermissionConfig: func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
				return nil
			},
			CSRSign: Sign,
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
		_, _, caCertBytes, err := getCA(client, true)
		if err == nil {
			caHashStamp := fmt.Sprintf("ca-hash-%x", sha256.Sum256(caCertBytes))
			observabilityConfig.Subject.OrganizationUnits = append(observabilityConfig.Subject.OrganizationUnits, caHashStamp)
		}

		kubeClientSignerConfigurations := agent.KubeClientSignerConfigurations(addonName, agentName)
		registrationConfig := append(kubeClientSignerConfigurations(cluster), observabilityConfig)
		return registrationConfig
	}
}
