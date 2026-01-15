// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"crypto/sha256"
	"fmt"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	_ *clusterv1.ManagedCluster,
	_ *addonapiv1alpha1.ManagedClusterAddOn,
) ([]runtime.Object, error) {
	return nil, nil
}

func (o *ObservabilityAgent) GetAgentAddonOptions() agent.AgentAddonOptions {
	signAdaptor := func(
		cluster *clusterv1.ManagedCluster,
		addon *addonapiv1alpha1.ManagedClusterAddOn,
		csr *certificatesv1.CertificateSigningRequest,
	) ([]byte, error) {
		res, err := Sign(o.client, csr)
		if err != nil {
			log.Error(err, "failed to sign")
			return nil, err
		}
		return res, nil
	}
	return agent.AgentAddonOptions{
		AddonName: addonName,
		Registration: &agent.RegistrationOption{
			CSRConfigurations: observabilitySignerConfigurations(o.client),
			CSRApproveCheck:   approve,
			PermissionConfig: func(_ *clusterv1.ManagedCluster, _ *addonapiv1alpha1.ManagedClusterAddOn) error {
				return nil
			},
			CSRSign: signAdaptor,
		},
		SupportedConfigGVRs: []schema.GroupVersionResource{
			{
				Group:    "addon.open-cluster-management.io",
				Version:  "v1alpha1",
				Resource: "addondeploymentconfigs",
			},
		},
	}
}

func observabilitySignerConfigurations(
	client client.Client,
) func(*clusterv1.ManagedCluster, *addonapiv1alpha1.ManagedClusterAddOn) ([]addonapiv1alpha1.RegistrationConfig, error) {
	return func(
		cluster *clusterv1.ManagedCluster,
		addon *addonapiv1alpha1.ManagedClusterAddOn,
	) ([]addonapiv1alpha1.RegistrationConfig, error) {
		observabilityConfig := addonapiv1alpha1.RegistrationConfig{
			SignerName: "open-cluster-management.io/observability-signer",
			Subject: addonapiv1alpha1.Subject{
				User:              "managed-cluster-observability",
				OrganizationUnits: []string{"acm"},
			},
		}

		kubeClientConfigs, err := agent.KubeClientSignerConfigurations(addonName, agentName)(cluster, addon)
		if err != nil {
			return nil, fmt.Errorf("failed to get kube client signer configurations: %w", err)
		}
		//nolint:gocritic // Creating new slice with additional config
		registrationConfigs := append(kubeClientConfigs, observabilityConfig)

		_, _, caCertBytes, caErr := getCA(client, true)
		if caErr == nil {
			caHashStamp := fmt.Sprintf("ca-hash-%x", sha256.Sum256(caCertBytes))
			for i := range registrationConfigs {
				registrationConfigs[i].Subject.OrganizationUnits = append(
					registrationConfigs[i].Subject.OrganizationUnits,
					caHashStamp,
				)
			}
		}

		return registrationConfigs, nil
	}
}
