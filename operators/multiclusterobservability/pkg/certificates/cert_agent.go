// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"context"
	"crypto/sha256"
	"fmt"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"open-cluster-management.io/addon-framework/pkg/agent"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
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
	ctx context.Context,
	cluster *clusterv1.ManagedCluster,
	addon *addonv1beta1.ManagedClusterAddOn,
) ([]runtime.Object, error) {
	return nil, nil
}

func (o *ObservabilityAgent) GetAgentAddonOptions() agent.AgentAddonOptions {
	signAdaptor := func(
		ctx context.Context,
		cluster *clusterv1.ManagedCluster,
		addon *addonv1beta1.ManagedClusterAddOn,
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
			Configurations:  observabilitySignerConfigurations(o.client),
			CSRApproveCheck: approve,
			PermissionConfig: func(
				ctx context.Context,
				cluster *clusterv1.ManagedCluster,
				addon *addonv1beta1.ManagedClusterAddOn,
			) error {
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
) func(context.Context, *clusterv1.ManagedCluster, *addonv1beta1.ManagedClusterAddOn) ([]agent.RegistrationConfig, error) {
	return func(
		ctx context.Context,
		cluster *clusterv1.ManagedCluster,
		addon *addonv1beta1.ManagedClusterAddOn,
	) ([]agent.RegistrationConfig, error) {
		observabilityConfig := &agent.CustomSignerRegistration{
			SignerName:        "open-cluster-management.io/observability-signer",
			User:              "managed-cluster-observability",
			OrganizationUnits: []string{"acm"},
		}

		kubeClientConfigs, err := agent.KubeClientSignerConfigurations(addonName, agentName)(ctx, cluster, addon)
		if err != nil {
			return nil, fmt.Errorf("failed to get kube client signer configurations: %w", err)
		}
		//nolint:gocritic // Creating new slice with additional config
		registrationConfigs := append(kubeClientConfigs, observabilityConfig)

		_, _, caCertBytes, caErr := getCA(client, true)
		if caErr == nil {
			caHashStamp := fmt.Sprintf("ca-hash-%x", sha256.Sum256(caCertBytes))
			for i := range registrationConfigs {
				switch c := registrationConfigs[i].(type) {
				case *agent.KubeClientRegistration:
					c.Groups = append(c.Groups, caHashStamp)
				case *agent.CustomSignerRegistration:
					c.OrganizationUnits = append(c.OrganizationUnits, caHashStamp)
				}
			}
		}

		return registrationConfigs, nil
	}
}
