// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"

	monv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	epv1 "github.com/open-cluster-management/endpoint-metrics-operator/pkg/apis/monitoring/v1"
	placev1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/controller/util"
)

const (
	epConfigName  = "endpoint-config"
	collectorType = "OCP_PROMETHEUS"
)

func createEndpointConfigCR(client client.Client,
	p *placev1.PlacementRule, scheme *runtime.Scheme,
	obsNamespace string, namespace string, cluster string) error {
	url, err := util.GetObsAPIUrl(client, obsNamespace)
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
							TargetLabel:  util.ClusterNameLabelKey,
							Replacement:  cluster,
						},
					},
				},
			},
		},
	}

	// Set PlacementRule instance as the owner and controller
	if err := controllerutil.SetControllerReference(p, ec, scheme); err != nil {
		return err
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
