// Copyright Contributors to the Open Cluster Management project
package status

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	oav1beta1 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

func init() {
	os.Setenv("UNIT_TEST", "true")
	s := scheme.Scheme
	_ = oav1beta1.AddToScheme(s)
}

func TestUpdateStatus(t *testing.T) {
	s, err := New(log.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create new Status struct: (%v)", err)
	}

	addon := &oav1beta1.ObservabilityAddon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: oav1beta1.ObservabilityAddonStatus{
			Conditions: []oav1beta1.StatusCondition{
				{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					Reason:             "Deployed",
					Message:            "Metrics collector deployed and functional",
					LastTransitionTime: metav1.NewTime(time.Now()),
				},
			},
		},
	}
	err = s.statusClient.Create(context.TODO(), addon)
	if err != nil {
		t.Fatalf("Failed to create observabilityAddon: (%v)", err)
	}

	err = s.UpdateStatus("Disabled", "Disabled", "enableMetrics is set to False")
	if err != nil {
		t.Fatalf("Failed to update status: (%v)", err)
	}

	err = s.UpdateStatus("Ready", "Deployed", "Metrics collector deployed and functional")
	if err != nil {
		t.Fatalf("Failed to update status: (%v)", err)
	}

	err = s.UpdateStatus("Ready", "Deployed", "Metrics collector deployed and updated")
	if err != nil {
		t.Fatalf("Failed to update status: (%v)", err)
	}
}
