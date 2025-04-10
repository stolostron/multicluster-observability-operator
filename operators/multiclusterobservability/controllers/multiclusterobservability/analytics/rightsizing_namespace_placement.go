package analytics

import (
	"context"

	"github.com/cloudflare/cfssl/log"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// createUpdatePlacement creates the Placement resource
func createUpdatePlacement(c client.Client, placementConfig clusterv1beta1.Placement) error {
	log.Info("RS - inside createUpdatePlacement")

	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rsPlacementName,
			Namespace: rsNamespace,
		},
	}
	key := types.NamespacedName{
		Namespace: rsNamespace,
		Name:      rsPlacementName,
	}

	err := c.Get(context.TODO(), key, placement)
	if errors.IsNotFound(err) {
		log.Info("RS - Placement not found, creating a new one", "Namespace", placement.Namespace, "Name", placement.Name)

		placement.Spec = placementConfig.Spec
		log.Info("RS - Updated Placement Spec")

		if err := c.Create(context.TODO(), placement); err != nil {
			log.Error(err, "Failed to create Placement")
			return err
		}

		log.Info("RS - Placement created", "Placement", placement.Name)
		return nil
	}

	if err != nil {
		log.Error(err, "RS - Unable to fetch Placement")
		return err
	}

	log.Info("RS - Placement exists, updating", "Namespace", placement.Namespace, "Name", placement.Name)

	placement.Spec = placementConfig.Spec
	log.Info("RS - Updated Placement Spec")

	if err := c.Update(context.TODO(), placement); err != nil {
		log.Error(err, "Failed to update Placement")
		return err
	}

	log.Info("RS - Placement updated", "Placement", placement.Name)
	return nil
}
