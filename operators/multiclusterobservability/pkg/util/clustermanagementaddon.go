// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"crypto/md5" // #nosec G401 G501 - Not used for cryptographic purposes
	"encoding/hex"
	"fmt"
	"net/url"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

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

// CalculateAddOnDeploymentConfigSpecHash computes a hash of the AddOnDeploymentConfig spec to track changes.
// This is a shared utility used by both ClusterManagementAddOn and ManagedClusterAddOn status updates.
func CalculateAddOnDeploymentConfigSpecHash(addonConfig *addonv1alpha1.AddOnDeploymentConfig) (string, error) {
	if addonConfig == nil {
		return "", nil
	}

	// Hash the spec portion of the AddOnDeploymentConfig
	hasher := md5.New() // #nosec G401 G501 - Not used for cryptographic purposes
	specData, err := yaml.Marshal(addonConfig.Spec)
	if err != nil {
		return "", fmt.Errorf("failed to marshal addon config spec: %w", err)
	}

	hasher.Write(specData)
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// updateClusterManagementAddOnStatus updates the ClusterManagementAddOn status with spec hash
func updateClusterManagementAddOnStatus(ctx context.Context, c client.Client, addonConfig *addonv1alpha1.AddOnDeploymentConfig) error {
	if addonConfig == nil {
		return nil
	}

	clusterMgmtAddon := &addonv1alpha1.ClusterManagementAddOn{}
	err := c.Get(ctx, types.NamespacedName{Name: ObservabilityController}, clusterMgmtAddon)
	if err != nil {
		return fmt.Errorf("failed to get clustermanagementaddon: %w", err)
	}

	specHash, err := CalculateAddOnDeploymentConfigSpecHash(addonConfig)
	if err != nil {
		return fmt.Errorf("failed to calculate spec hash: %w", err)
	}

	// Save original CMA before modifying for Patch operation
	// Use Patch instead of Update to avoid overwriting status fields set by addon-framework's cmaconfig controller
	// The framework uses PatchStatus to update spec hashes, and we should preserve other status updates
	originalCMA := clusterMgmtAddon.DeepCopy()

	// Update status with desired config spec hash
	// Find the configuration entry for AddOnDeploymentConfig and update its desiredConfig
	found := false
	for i, configRef := range clusterMgmtAddon.Status.DefaultConfigReferences {
		if configRef.ConfigGroupResource.Group == AddonGroup && configRef.ConfigGroupResource.Resource == AddonDeploymentConfigResource {
			if clusterMgmtAddon.Status.DefaultConfigReferences[i].DesiredConfig == nil {
				clusterMgmtAddon.Status.DefaultConfigReferences[i].DesiredConfig = &addonv1alpha1.ConfigSpecHash{}
			}
			clusterMgmtAddon.Status.DefaultConfigReferences[i].DesiredConfig.SpecHash = specHash
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("DefaultConfigReference not found in CMA status for config %s/%s - framework may not have created it yet, will retry", addonConfig.Namespace, addonConfig.Name)
	}

	if equality.Semantic.DeepEqual(originalCMA.Status.DefaultConfigReferences, clusterMgmtAddon.Status.DefaultConfigReferences) {
		log.V(1).Info("DefaultConfigReferences unchanged, skipping status update")
		return nil
	}

	// Use Patch instead of Update to preserve other status fields updated by the framework
	if err := c.Status().Patch(ctx, clusterMgmtAddon, client.MergeFrom(originalCMA)); err != nil {
		return fmt.Errorf("failed to update clustermanagementaddon status: %w", err)
	}

	log.Info("Updated ClusterManagementAddOn status with spec hash", "hash", specHash, "config", addonConfig.Namespace+"/"+addonConfig.Name)
	return nil
}

// UpdateClusterManagementAddOnSpecHash is a public function that can be called to update
// the ClusterManagementAddOn status with spec hash for a given AddOnDeploymentConfig.
// This should be called whenever AddOnDeploymentConfigs are created or updated.
func UpdateClusterManagementAddOnSpecHash(ctx context.Context, c client.Client, addonConfig *addonv1alpha1.AddOnDeploymentConfig) error {
	return updateClusterManagementAddOnStatus(ctx, c, addonConfig)
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

	// With addon-framework v0.12.0 upgrade, we need to ensure the self-management annotation is present
	// to properly handle spec hash updates for AddOnDeploymentConfigs
	if found.Annotations == nil {
		found.Annotations = map[string]string{}
	}

	// Ensure the self-management annotation is set
	if found.Annotations[addonv1alpha1.AddonLifecycleAnnotationKey] != addonv1alpha1.AddonLifecycleSelfManageAnnotationValue {
		found.Annotations[addonv1alpha1.AddonLifecycleAnnotationKey] = addonv1alpha1.AddonLifecycleSelfManageAnnotationValue

		log.Info("Setting self-management annotation on observability-controller clustermanagementaddon")
		if err := c.Update(ctx, found); err != nil {
			return nil, fmt.Errorf("failed to update clustermanagementaddon to set self-management annotation: %w", err)
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
