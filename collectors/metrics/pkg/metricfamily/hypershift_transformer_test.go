// Copyright Contributors to the Open Cluster Management project
package metricfamily

import (
	"context"
	"os"
	"testing"

	"github.com/go-kit/log"
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	prom "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	metricsName         = "test_metrics"
	idLabel             = "_id"
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

func init() {
	os.Setenv("UNIT_TEST", "true")
	s := scheme.Scheme
	hyperv1.AddToScheme(s)
}

func TestTransform(t *testing.T) {
	c := fake.NewFakeClient(hCluster)

	l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	labels := map[string]string{
		"cluster":   "test-cluster",
		"clusterID": "test-clusterID",
	}
	h, err := NewHypershiftTransformer(l, c, labels)
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
