// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package metricfamily

import (
	"context"
	"log/slog"
	"os"
	"testing"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	prom "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	metricsName = "test_metrics"
	idLabel     = "_id"
	// id                  = "test_id"
	hostedClusterName   = "test-hosted-cluster"
	hostedClusterID     = "test-hosted-cluster-id"
	testLabel           = "test_label"
	testLabelValue      = "test-label-value"
	hostedClusterName_1 = "test-hosted-cluster-1"
	hostedClusterID_1   = "test-hosted-cluster-id-1"
)

var (
	hCluster = &hyperv1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: hostedClusterName,
		},
		Spec: hyperv1.HostedClusterSpec{
			ClusterID: hostedClusterID,
		},
	}
	hCluster_1 = &hyperv1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: hostedClusterName_1,
		},
		Spec: hyperv1.HostedClusterSpec{
			ClusterID: hostedClusterID_1,
		},
	}
)

func TestTransform(t *testing.T) {
	s := scheme.Scheme
	if err := hyperv1.AddToScheme(s); err != nil {
		t.Fatal("couldn't add hyperv1 to scheme")
	}

	c := fake.NewClientBuilder().WithRuntimeObjects(hCluster).Build()

	l := slog.New(slog.NewTextHandler(os.Stderr, nil))
	labels := map[string]string{
		"cluster":   "test-cluster",
		"clusterID": "test-clusterID",
	}

	h, err := NewHypershiftTransformer(c, l, labels)
	if err != nil {
		t.Fatal("Failed to new HyperShiftTransformer", err)
	}

	family := &prom.MetricFamily{
		Name: &metricsName,
		Metric: []*prom.Metric{
			{
				Label: []*prom.LabelPair{
					{
						Name:  &idLabel,
						Value: &hostedClusterID,
					},
					{
						Name:  &testLabel,
						Value: &testLabelValue,
					},
				},
			},
		},
	}
	_, err = h.Transform(family)
	if err != nil {
		t.Fatal("Failed to transform metrics", err)
	}
	family.Metric = append(family.Metric, &prom.Metric{
		Label: []*prom.LabelPair{
			{
				Name:  &idLabel,
				Value: &hostedClusterID_1,
			},
		},
	})
	err = c.Create(context.TODO(), hCluster_1, &client.CreateOptions{})
	if err != nil {
		t.Fatal("Failed to create HostedCluster", err)
	}
	_, err = h.Transform(family)
	if err != nil {
		t.Fatal("Failed to transform metrics", err)
	}
}
