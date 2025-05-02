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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// isPlatformFeatureConfigured checks if the Platform feature is enabled
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

// isCRDRegistered is injectable for testing
var isCRDRegistered = func(gvk schema.GroupVersionKind) bool {
	config, err := rest.InClusterConfig()
	if err != nil {
		return true // assume true in dev/test
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return true
	}
	apiResourceLists, err := discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return false
	}
	for _, resource := range apiResourceLists.APIResources {
		if resource.Kind == gvk.Kind {
			return true
		}
	}
	return false
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
		resourcesToDelete = commonResources
	} else {
		resourcesToDelete = append(commonResources,
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: rsConfigMapName, Namespace: config.GetDefaultNamespace()}},
		)
	}

	for _, resource := range resourcesToDelete {
		gvk := resource.GetObjectKind().GroupVersionKind()
		if gvk.Empty() {
			switch resource.(type) {
			case *policyv1.PlacementBinding:
				gvk = schema.GroupVersionKind{Group: "policy.open-cluster-management.io", Version: "v1", Kind: "PlacementBinding"}
			case *policyv1.Policy:
				gvk = schema.GroupVersionKind{Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy"}
			}
		}

		if !isCRDRegistered(gvk) {
			log.Info("CRD not registered for resource, skipping deletion", "name", resource.GetName(), "gvk", gvk.String())
			continue
		}

		err := c.Delete(ctx, resource)
		if err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete resource", "name", resource.GetName())
		}
	}

	log.Info("RS - Cleanup success.")
}
