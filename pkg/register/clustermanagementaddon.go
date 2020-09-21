// Copyright (c) 2020 Red Hat, Inc.

package register

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cfssl/log"
	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//"github.com/prometheus/common/log"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Constants for CLusterManagementAddons Names
const (
	ObservabilityController = "observability-controller"
	DisplayName             = "Observability Controller"
	Description             = "Manages Observability components."
	CRDName                 = "klusterletaddonconfigs.agent.open-cluster-management.io"
)

//  ObservabilityController CreateClusterManagementAddon
func CreateClusterManagementAddon(c client.Client) {
	addOnFound := false
	for !addOnFound {
		clusterManagementAddon := &addonv1alpha1.ClusterManagementAddOn{}
		if err := c.Get(context.TODO(), types.NamespacedName{Name: ObservabilityController}, clusterManagementAddon); err != nil {
			if errors.IsNotFound(err) {
				clusterManagementAddon := newClusterManagementAddon()
				// Create OCP client
				ocpClient, err := util.CreateOCPClient()
				if err != nil {
					log.Error(err, "Failed to create the OpenShift client")
					return nil
				}
				if err := c.Create(context.TODO(), clusterManagementAddon); err != nil {
					log.Error(err, fmt.Sprintf("Failed to create observability-controller clustermanagementaddon "))
					break
				}
				log.Info(fmt.Sprintf("Created observability-controller clustermanagementaddon"))
				break
			}
			switch err.(type) {
			case *cache.ErrCacheNotStarted:
				time.Sleep(time.Second)
				continue
			default:
				log.Error(err, fmt.Sprintf("Cannot create observability-controller clustermanagementaddon"))
				break
			}
		}
		log.Info(fmt.Sprintf("observability-controller clustermanagementaddon is present "))
		addOnFound = true
	}
}

func newClusterManagementAddon() *addonv1alpha1.ClusterManagementAddOn {
	return &addonv1alpha1.ClusterManagementAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ClusterManagementAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			AddOnMeta: addonv1alpha1.AddOnMeta{
				DisplayName: DisplayName,
				Description: Description,
			},
			AddOnConfiguration: addonv1alpha1.ConfigCoordinates{
				CRDName: CRDName,
			},
		},
	}
}
