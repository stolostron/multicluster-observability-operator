// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rsvirtualization

import (
	"context"
	"fmt"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	rsutility "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability/analytics/rs-utility"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// Virtualization-specific resource names
	PlacementBindingName     = "rs-virt-policyset-binding"
	PlacementName            = "rs-virt-placement"
	PrometheusRulePolicyName = "rs-virt-prom-rules-policy"
	PrometheusRuleName       = "acm-rs-virt-prometheus-rules"
	ConfigMapName            = "rs-virt-config"

	// Common constants
	MonitoringNamespace = "openshift-monitoring"
	DefaultNamespace    = "open-cluster-management-global-set"
)

var (
	// State variables - exported for testing
	Namespace = DefaultNamespace
	Enabled   = false

	log = logf.Log.WithName("rs-virtualization")
)

// HandleRightSizing handles the virtualization right-sizing functionality
func HandleRightSizing(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability) error {
	log.V(1).Info("rs - handling virtualization right-sizing")

	// Get right-sizing virtualization configuration
	isRightSizingVirtEnabled, newBinding := GetRightSizingVirtualizationConfig(mco)

	// Set to default namespace if not given
	if newBinding == "" {
		newBinding = DefaultNamespace
	}

	// Check if right-sizing virtualization feature enabled or not
	// If disabled then cleanup related resources
	if !isRightSizingVirtEnabled {
		log.V(1).Info("rs - virtualization rs feature not enabled")
		CleanupRSVirtualizationResources(ctx, c, Namespace, false)
		Namespace = newBinding
		Enabled = false
		return nil
	}

	// Set the flag if namespaceBindingUpdated
	namespaceBindingUpdated := Namespace != newBinding && Enabled

	// Set enabled flag which will be used for checking namespaceBindingUpdated condition next time
	Enabled = true

	// Retrieve the existing namespace as existingNamespace and update the new namespace
	existingNamespace := Namespace
	Namespace = newBinding

	// Creating configmap with default values
	if err := EnsureRSVirtualizationConfigMapExists(ctx, c); err != nil {
		return err
	}

	if namespaceBindingUpdated {
		// Clean up resources except config map to update NamespaceBinding
		CleanupRSVirtualizationResources(ctx, c, existingNamespace, true)

		// Get configmap
		cm := &corev1.ConfigMap{}
		if err := c.Get(ctx, client.ObjectKey{Name: ConfigMapName, Namespace: config.GetDefaultNamespace()}, cm); err != nil {
			return fmt.Errorf("rs - failed to get existing configmap: %w", err)
		}

		// Get configmap data into specified structure
		configData, err := GetRightSizingVirtualizationConfigData(cm)
		if err != nil {
			return fmt.Errorf("rs - failed to extract config data: %w", err)
		}

		// If NamespaceBinding has been updated apply the Policy Placement Placementbinding again
		if err := ApplyRSVirtualizationConfigMapChanges(ctx, c, configData); err != nil {
			return fmt.Errorf("rs - failed to apply configmap changes: %w", err)
		}
	}

	log.Info("rs - virtualization right-sizing handling completed")
	return nil
}

// GetRightSizingVirtualizationConfig gets the virtualization right-sizing configuration
func GetRightSizingVirtualizationConfig(mco *mcov1beta2.MultiClusterObservability) (bool, string) {
	isRightSizingEnabled := false
	namespaceBinding := ""
	if rsutility.IsPlatformFeatureConfigured(mco) {
		isRightSizingEnabled = mco.Spec.Capabilities.Platform.Analytics.VirtualizationRightSizingRecommendation.Enabled
		namespaceBinding = mco.Spec.Capabilities.Platform.Analytics.VirtualizationRightSizingRecommendation.NamespaceBinding
	}
	return isRightSizingEnabled, namespaceBinding
}

// CleanupRSVirtualizationResources cleans up the resources created for virtualization right-sizing
func CleanupRSVirtualizationResources(ctx context.Context, c client.Client, namespace string, bindingUpdated bool) {
	log.V(1).Info("rs - cleaning up virtualization resources if exist")

	var resourcesToDelete []client.Object
	commonResources := []client.Object{
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: PlacementBindingName, Namespace: namespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: PlacementName, Namespace: namespace}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: PrometheusRulePolicyName, Namespace: namespace}},
	}

	if bindingUpdated {
		// If NamespaceBinding has been updated apply the Policy Placement Placementbinding again
		resourcesToDelete = commonResources
	} else {
		resourcesToDelete = append(commonResources,
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName, Namespace: config.GetDefaultNamespace()}},
		)
	}

	// Delete related resources
	for _, resource := range resourcesToDelete {
		err := c.Delete(ctx, resource)
		if err != nil && !errors.IsNotFound(err) {
			log.Error(err, "rs - failed to delete resource", "name", resource.GetName())
		}
	}
	log.Info("rs - cleanup success")
}
