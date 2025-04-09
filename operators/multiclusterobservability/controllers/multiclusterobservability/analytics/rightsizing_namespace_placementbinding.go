package analytics

import (
	"context"

	"github.com/cloudflare/cfssl/log"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// createPlacementBinding creates the PlacementBinding resource
func createPlacementBinding(c client.Client) error {
	log.Info("RS - PlacementBinding creation started")
	placementBinding := &policyv1.PlacementBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPlacementBindingName,
			Namespace: rsNamespace,
		},
	}
	err := c.Get(context.TODO(), types.NamespacedName{
		Namespace: placementBinding.Namespace,
		Name:      placementBinding.Name,
	}, placementBinding)
	log.Info("RS - fetch PlacementBinding completed2")

	if err != nil && errors.IsNotFound(err) {

		log.Info("RS - PlacementBinding not found, creating a new one",
			"Namespace", placementBinding.Namespace,
			"Name", placementBinding.Name,
		)
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "RS - Unable to fetch PlacementBinding")
			return err
		}

		placementBinding.PlacementRef = policyv1.PlacementSubject{
			Name:     rsPlacementName,
			APIGroup: "cluster.open-cluster-management.io",
			Kind:     "Placement",
		}
		placementBinding.Subjects = []policyv1.Subject{
			{
				Name:     rsPrometheusRulePolicyName,
				APIGroup: "policy.open-cluster-management.io",
				Kind:     "Policy",
			},
		}

		if err = c.Create(context.TODO(), placementBinding); err != nil {
			log.Error(err, "Failed to create Placement")
			return err
		}
		log.Info("RS - Create PlacementBinding completed", "PlacementBinding", rsPlacementBindingName)
	}
	log.Info("RS - PlacementBinding creation completed")
	return nil
}
