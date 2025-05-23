// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"fmt"
	"net/url"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

const (
	ObservabilityController       = "observability-controller" // #nosec G101 -- Not a hardcoded credential.
	AddonGroup                    = "addon.open-cluster-management.io"
	AddonDeploymentConfigResource = "addondeploymentconfigs"
	grafanaLink                   = "/d/2b679d600f3b9e7676a7c5ac3643d448/acm-clusters-overview"
)

type clusterManagementAddOnSpec struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	CRDName     string `json:"crdName"`
}

func CreateClusterManagementAddon(ctx context.Context, c client.Client) (
	*addonv1alpha1.ClusterManagementAddOn, error) {
	clusterManagementAddon, err := newClusterManagementAddon(c)
	if err != nil {
		return nil, err
	}

	found := &addonv1alpha1.ClusterManagementAddOn{}
	err = c.Get(ctx, types.NamespacedName{Name: ObservabilityController}, found)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating observability-controller clustermanagementaddon")
			if err := c.Create(ctx, clusterManagementAddon); err != nil {
				return nil, fmt.Errorf("failed to create observability-controller clustermanagementaddon: %w", err)
			}
			return clusterManagementAddon, nil
		}
		return nil, fmt.Errorf("cannot create observability-controller clustermanagementaddon: %w", err)
	}

	// With addon-framework v0.12.0 upgrade, the lifecycle annotation should not be present on clustermanagementaddon,
	// as otherwise addon-manager-controller does not correctly precess the addon.
	// Remove addon.open-cluster-management.io/lifecycle annotation if present.
	if found.Annotations != nil {
		if _, exists := found.Annotations[addonv1alpha1.AddonLifecycleAnnotationKey]; exists {
			delete(found.Annotations, addonv1alpha1.AddonLifecycleAnnotationKey)

			log.Info("Removing addon.open-cluster-management.io/lifecycle annotation from observability-controller clustermanagementaddon")
			if err := c.Update(ctx, found); err != nil {
				return nil, fmt.Errorf("failed to update clustermanagementaddon to remove lifecycle annotation: %w", err)
			}
		}
	}

	return found, nil
}

func DeleteClusterManagementAddon(ctx context.Context, client client.Client) error {
	clustermanagementaddon := &addonv1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
		},
	}
	err := client.Delete(ctx, clustermanagementaddon)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete clustermanagementaddon %s: %w", clustermanagementaddon.Name, err)
	}

	log.Info("ClusterManagementAddon deleted", "name", ObservabilityController)
	return nil
}

func newClusterManagementAddon(c client.Client) (*addonv1alpha1.ClusterManagementAddOn, error) {
	host, err := config.GetRouteHost(c, config.GrafanaRouteName, config.GetDefaultNamespace())
	if err != nil {
		return nil, fmt.Errorf("failed to get host route: %w", err)
	}
	grafanaUrl := url.URL{
		Scheme: "https",
		Host:   host,
		Path:   grafanaLink,
	}
	clusterManagementAddOnSpec := clusterManagementAddOnSpec{
		DisplayName: "Observability Controller",
		Description: "Manages Observability components.",
		CRDName:     "observabilityaddons.observability.open-cluster-management.io",
	}
	return &addonv1alpha1.ClusterManagementAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ClusterManagementAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
			Annotations: map[string]string{
				"console.open-cluster-management.io/launch-link":      grafanaUrl.String(),
				"console.open-cluster-management.io/launch-link-text": "Grafana",
				addonv1alpha1.AddonLifecycleAnnotationKey:             addonv1alpha1.AddonLifecycleSelfManageAnnotationValue,
			},
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			AddOnMeta: addonv1alpha1.AddOnMeta{
				DisplayName: clusterManagementAddOnSpec.DisplayName,
				Description: clusterManagementAddOnSpec.Description,
			},
			InstallStrategy: addonv1alpha1.InstallStrategy{
				Type: addonv1alpha1.AddonInstallStrategyManual,
			},
			AddOnConfiguration: addonv1alpha1.ConfigCoordinates{
				CRDName: clusterManagementAddOnSpec.CRDName,
			},
			SupportedConfigs: []addonv1alpha1.ConfigMeta{
				{
					ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
						Group:    AddonGroup,
						Resource: AddonDeploymentConfigResource,
					},
				},
			},
		},
	}, nil
}
