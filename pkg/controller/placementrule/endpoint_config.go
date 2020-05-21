// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"

	monv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epv1 "github.com/open-cluster-management/endpoint-metrics-operator/pkg/apis/monitoring/v1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/controller/util"
)

const (
	epConfigName  = "endpoint_config"
	collectorType = "OCP_PROMETHEUS"
)

func createEndpointConfigCR(client client.Client, namespace string, cluster string) error {
	url, err := util.GetObsAPIUrl(client, namespace)
	if err != nil {
		return err
	}
	ec := &epv1.EndpointMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Name:      epConfigName,
			Namespace: namespace,
		},
		Spec: epv1.EndpointMetricsSpec{
			GlobalConfig: epv1.GlobalConfigSpec{
				SeverURL: url,
			},
			MetricsCollectorList: []epv1.MetricsCollectorSpec{
				{
					Enable: true,
					Type:   collectorType,
					RelabelConfigs: []monv1.RelabelConfig{
						{
							SourceLabels: []string{"__name__"},
							TargetLabel:  util.LabelKey,
							Replacement:  cluster,
						},
					},
				},
			},
		},
	}
	found := &epv1.EndpointMetrics{}
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

	log.Info("endponitmetrics already existed", "namespace", namespace)
	return nil
}
