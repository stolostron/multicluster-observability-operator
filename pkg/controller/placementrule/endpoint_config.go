// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"

	monv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epv1alpha1 "github.com/open-cluster-management/multicluster-observability-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
)

const (
	epConfigName  = "endpoint-config"
	collectorType = "OCP_PROMETHEUS"
)

func deleteEndpointConfigCR(client client.Client, namespace string) error {
	found := &epv1alpha1.EndpointMonitoring{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: epConfigName, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to check endpoint config cr", "namespace", namespace)
		return err
	}
	err = client.Delete(context.TODO(), found)
	if err != nil {
		log.Error(err, "Failed to delete endpointmonitoring", "namespace", namespace)
	}
	log.Info("endpointmonitoring is deleted", "namespace", namespace)
	return err
}

func createEndpointConfigCR(client client.Client, obsNamespace string, namespace string, cluster string) error {
	url, err := config.GetObsAPIUrl(client, obsNamespace)
	if err != nil {
		return err
	}
	ec := &epv1alpha1.EndpointMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name:      epConfigName,
			Namespace: namespace,
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Spec: epv1alpha1.EndpointMonitoringSpec{
			GlobalConfig: epv1alpha1.GlobalConfigSpec{
				SeverURL: url,
			},
			MetricsCollectorList: []epv1alpha1.MetricsCollectorSpec{
				{
					Enable: true,
					Type:   collectorType,
					RelabelConfigs: []monv1.RelabelConfig{
						{
							SourceLabels: []string{"__name__"},
							TargetLabel:  config.GetClusterNameLabelKey(),
							Replacement:  cluster,
						},
					},
				},
			},
		},
	}
	found := &epv1alpha1.EndpointMonitoring{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: epConfigName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating endpoint config cr", "namespace", namespace)
		err = client.Create(context.TODO(), ec)
		if err != nil {
			log.Error(err, "Failed to create endpoint config cr")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check endpoint config cr")
		return err
	}

	log.Info("endponitmetrics already existed/unchanged", "namespace", namespace)
	return nil
}
