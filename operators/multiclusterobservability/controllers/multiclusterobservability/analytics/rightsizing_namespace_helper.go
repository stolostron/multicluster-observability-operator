package analytics

import (
	"context"

	"github.com/cloudflare/cfssl/log"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// isRightSizingNamespaceEnabled checks if the right-sizing namespace analytics feature is enabled in the provided MCO configuration.
func isRightSizingNamespaceEnabled(mco *mcov1beta2.MultiClusterObservability) bool {
	return mco.Spec.Capabilities != nil &&
		mco.Spec.Capabilities.Platform != nil &&
		mco.Spec.Capabilities.Platform.Analytics != nil &&
		mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation != nil &&
		mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.Enabled
}

func cleanupRSNamespaceResources(c client.Client, namespace string) {
	log.Info("RS - Cleaning up NamespaceRightSizing resources")

	// Define all objects to delete
	resourcesToDelete := []client.Object{
		&policyv1.PlacementBinding{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementBindingName, Namespace: namespace}},
		&clusterv1beta1.Placement{ObjectMeta: metav1.ObjectMeta{Name: rsPlacementName, Namespace: namespace}},
		&policyv1.Policy{ObjectMeta: metav1.ObjectMeta{Name: rsPrometheusRulePolicyName, Namespace: namespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: rsConfigMapName, Namespace: config.GetDefaultNamespace()}},
	}

	// Iterate and delete each resource if it exists
	for _, resource := range resourcesToDelete {
		err := c.Delete(context.TODO(), resource)
		if err != nil {
			if errors.IsNotFound(err) {
				// Do nothing if resource is not found
				continue
			}
			log.Error(err, "Failed to delete resource", "Resource", resource.GetObjectKind().GroupVersionKind().Kind, "Name", resource.GetName())
		} else {
			log.Info("Deleted resource successfully", "Resource", resource.GetObjectKind().GroupVersionKind().Kind, "Name", resource.GetName())
		}
	}

	// Set default namespace again
	rsNamespace = rsDefaultNamespace
}
