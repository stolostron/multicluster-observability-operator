// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"testing"
	"time"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func init() {
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	config.SetMonitoringCRName(name)
}

func newDeployment(name string) *appv1.Deployment {
	return &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		Spec: appv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"label": "value"},
				},
			},
		},
		Status: appv1.DeploymentStatus{
			ReadyReplicas: 1,
		},
	}
}

func TestOnAdd(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	caSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              serverCACerts,
			Namespace:         namespace,
			CreationTimestamp: metav1.Date(2020, time.January, 2, 0, 0, 0, 0, time.UTC),
		},
	}
	config.SetOperandNames(t.Context(), c)
	onAdd(c)(caSecret)
	c = fake.NewClientBuilder().WithRuntimeObjects(newDeployment(name + "-observatorium-api")).Build()
	dep := &appv1.Deployment{}
	caSecret.Name = clientCACerts
	onAdd(c)(caSecret)
	c.Get(t.Context(),
		types.NamespacedName{Name: name + "-observatorium-api", Namespace: namespace},
		dep)
	if dep.Spec.Template.ObjectMeta.Labels[restartLabel] == "" {
		t.Fatalf("Failed to inject restart label")
	}
}

func TestOnDelete(t *testing.T) {
	caSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverCACerts,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("new cert-"),
		},
	}
	deletCaSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverCACerts,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("old cert"),
		},
	}
	c := fake.NewClientBuilder().WithRuntimeObjects(caSecret, getMco()).Build()
	onDelete(c)(deletCaSecret)
	c.Get(t.Context(), types.NamespacedName{Name: serverCACerts, Namespace: namespace}, caSecret)
	data := string(caSecret.Data["tls.crt"])
	if data != "new cert-old cert" {
		t.Fatalf("deleted cert not added back: %s", data)
	}
}

func TestOnUpdate(t *testing.T) {
	certSecret := getExpiredCertSecret()
	oldCertLength := len(certSecret.Data["tls.crt"])
	c := fake.NewClientBuilder().WithRuntimeObjects(certSecret).Build()
	onUpdate(c, true)(certSecret, certSecret)
	certSecret.Name = clientCACerts
	onUpdate(c, true)(certSecret, certSecret)
	certSecret.Name = grafanaCerts
	onUpdate(c, true)(certSecret, certSecret)
	certSecret.Name = serverCerts
	onUpdate(c, true)(certSecret, certSecret)
	c.Get(t.Context(), types.NamespacedName{Name: serverCACerts, Namespace: namespace}, certSecret)
	if len(certSecret.Data["tls.crt"]) <= oldCertLength {
		t.Fatal("certificate not renewed correctly")
	}
}
