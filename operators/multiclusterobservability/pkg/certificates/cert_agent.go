// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package certificates

import (
	"k8s.io/apimachinery/pkg/runtime"

	"open-cluster-management.io/addon-framework/pkg/agent"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const (
	addonName = "observability-controller"
	agentName = "observability"
)

type ObservabilityAgent struct{}

func (o *ObservabilityAgent) Manifests(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) ([]runtime.Object, error) {
	return nil, nil
}

func (o *ObservabilityAgent) GetAgentAddonOptions() agent.AgentAddonOptions {
	return agent.AgentAddonOptions{
		AddonName: addonName,
		Registration: &agent.RegistrationOption{
			CSRConfigurations: observabilitySignerConfigurations(),
			CSRApproveCheck:   approve,
			PermissionConfig: func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
				return nil
			},
			CSRSign: sign,
		},
	}
}

func observabilitySignerConfigurations() func(cluster *clusterv1.ManagedCluster) []addonapiv1alpha1.RegistrationConfig {
	return func(cluster *clusterv1.ManagedCluster) []addonapiv1alpha1.RegistrationConfig {
		observabilityConfig := addonapiv1alpha1.RegistrationConfig{
			SignerName: "open-cluster-management.io/observability-signer",
			Subject: addonapiv1alpha1.Subject{
				User:              "managed-cluster-observability",
				OrganizationUnits: []string{"acm"},
			},
		}
		return append(agent.KubeClientSignerConfigurations(addonName, agentName)(cluster), observabilityConfig)
	}
}
