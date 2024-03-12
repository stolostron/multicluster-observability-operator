// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"fmt"
	"net/url"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

const (
	ObservabilityController       = "observability-controller" // #nosec G101  -- false positive, not a hardcoded credential
	AddonGroup                    = "addon.open-cluster-management.io"
	AddonDeploymentConfigResource = "addondeploymentconfigs"
	grafanaLink                   = "/d/2b679d600f3b9e7676a7c5ac3643d448/acm-clusters-overview"
)

type clusterManagementAddOnSpec struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	CRDName     string `json:"crdName"`
}

func CreateClusterManagementAddon(c client.Client, isStandalone bool) (
	*addonv1alpha1.ClusterManagementAddOn, error) {
	clusterManagementAddon, err := newClusterManagementAddon(c, isStandalone)
	if err != nil {
		return nil, err
	}
	found := &addonv1alpha1.ClusterManagementAddOn{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: ObservabilityController}, found)
	if err != nil && errors.IsNotFound(err) {
		if err := c.Create(context.TODO(), clusterManagementAddon); err != nil {
			log.Error(err, "Failed to create observability-controller clustermanagementaddon ")
			return nil, err
		}
		log.Info("Created observability-controller clustermanagementaddon")
		return clusterManagementAddon, nil
	} else if err != nil {
		log.Error(err, "Cannot create observability-controller clustermanagementaddon")
		return nil, err
	}

	log.Info(fmt.Sprintf("%s clustermanagementaddon is present ", ObservabilityController))
	return found, nil
}

func DeleteClusterManagementAddon(client client.Client) error {
	clustermanagementaddon := &addonv1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: ObservabilityController,
		},
	}
	err := client.Delete(context.TODO(), clustermanagementaddon)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete clustermanagementaddon", "name", ObservabilityController)
		return err
	}
	log.Info("ClusterManagementAddon deleted", "name", ObservabilityController)
	return nil
}

func newClusterManagementAddon(c client.Client, isStandalone bool) (*addonv1alpha1.ClusterManagementAddOn, error) {
	host, err := config.GetRouteHost(c, config.GrafanaRouteName, config.GetDefaultNamespace())
	if err != nil {
		return nil, err
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
