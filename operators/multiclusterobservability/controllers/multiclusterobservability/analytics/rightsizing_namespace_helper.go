// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package analytics

import (
	"context"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// isPlatformFeatureConfigured checks if the Platform  feature is enabled
func isPlatformFeatureConfigured(mco *mcov1beta2.MultiClusterObservability) bool {
	return mco.Spec.Capabilities != nil &&
		mco.Spec.Capabilities.Platform != nil
}

// Get rightsizing namespace configuration
func getRightSizingNamespaceConfig(mco *mcov1beta2.MultiClusterObservability) (bool, string) {

	isRightSizingEnabled := false
	namespaceBinding := ""
	if isPlatformFeatureConfigured(mco) {
		isRightSizingEnabled = mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.Enabled
		namespaceBinding = mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.NamespaceBinding
	}
	return isRightSizingEnabled, namespaceBinding
}

// cleanupRSNamespaceResources cleans up the resources created for namespace right-sizing
func cleanupRSNamespaceResources(ctx context.Context, c client.Client, namespace string, bindingUpdated bool) {
	log.Info("RS - Cleaning up NamespaceRightSizing resources if exist")

	var resourcesToDelete []client.Object
	commonResources := []client.Object{
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementBindingName, Namespace: namespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementName, Namespace: namespace}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: rsPrometheusRulePolicyName, Namespace: namespace}},
	}

	if bindingUpdated {
		// If NamespaceBinding has been updated apply the Policy Placement Placementbinding again
		resourcesToDelete = commonResources
	} else {
		resourcesToDelete = append(commonResources,
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: rsConfigMapName, Namespace: config.GetDefaultNamespace()}},
		)
	}

	for _, resource := range resourcesToDelete {
		err := c.Delete(ctx, resource)
		if err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete resource", "name", resource.GetName())
		}
	}
	log.Info("RS - Cleanup success.")
}
