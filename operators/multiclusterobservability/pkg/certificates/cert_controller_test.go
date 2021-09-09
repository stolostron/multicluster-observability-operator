// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package certificates

import (
	"context"
	"testing"
	"time"

	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

func init() {
	//logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stdout)))

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
	c := fake.NewFakeClient()
	caSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              serverCACerts,
			Namespace:         namespace,
			CreationTimestamp: metav1.Date(2020, time.January, 2, 0, 0, 0, 0, time.UTC),
		},
	}
	config.SetOperandNames(c)
	onAdd(c)(caSecret)
	c = fake.NewFakeClient(newDeployment(name+"-rbac-query-proxy"),
		newDeployment(name+"-observatorium-api"))
	onAdd(c)(caSecret)
	dep := &appv1.Deployment{}
	c.Get(context.TODO(),
		types.NamespacedName{Name: name + "-rbac-query-proxy", Namespace: namespace},
		dep)
	if dep.Spec.Template.ObjectMeta.Labels[restartLabel] == "" {
		t.Fatalf("Failed to inject restart label")
	}
	caSecret.Name = clientCACerts
	onAdd(c)(caSecret)
	c.Get(context.TODO(),
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
	c := fake.NewFakeClient(caSecret, getMco())
	onDelete(c)(deletCaSecret)
	c.Get(context.TODO(), types.NamespacedName{Name: serverCACerts, Namespace: namespace}, caSecret)
	data := string(caSecret.Data["tls.crt"])
	if data != "new cert-old cert" {
		t.Fatalf("deleted cert not added back: %s", data)
	}
}

func TestOnUpdate(t *testing.T) {
	certSecret := getExpiredCertSecret()
	oldCertLength := len(certSecret.Data["tls.crt"])
	c := fake.NewFakeClient(certSecret)
	onUpdate(c, true)(certSecret, certSecret)
	certSecret.Name = clientCACerts
	onUpdate(c, true)(certSecret, certSecret)
	certSecret.Name = grafanaCerts
	onUpdate(c, true)(certSecret, certSecret)
	certSecret.Name = serverCerts
	onUpdate(c, true)(certSecret, certSecret)
	c.Get(context.TODO(), types.NamespacedName{Name: serverCACerts, Namespace: namespace}, certSecret)
	if len(certSecret.Data["tls.crt"]) <= oldCertLength {
		t.Fatal("certificate not renewed correctly")
	}
}
