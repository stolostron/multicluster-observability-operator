// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package openshift_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	ocinfrav1 "github.com/openshift/api/config/v1"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/openshift"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testClusterID         = "kind-cluster-id"
	hostedClusterName     = "test-hosted-cluster"
	hosteClusterNamespace = "clusters"
)

var (
	cv = &ocinfrav1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Spec: ocinfrav1.ClusterVersionSpec{
			ClusterID: testClusterID,
		},
	}
)

func init() {
	s := scheme.Scheme
	hyperv1.AddToScheme(s)
	promv1.AddToScheme(s)
}

func TestCreateDeleteCAConfigmap(t *testing.T) {
	ctx := context.TODO()
	c := fake.NewClientBuilder().Build()
	namespace := "test-ns"
	err := openshift.CreateCAConfigmap(ctx, c, namespace)
	if err != nil {
		t.Fatalf("Failed to create CA configmap: (%v)", err)
	}
	err = openshift.DeleteCAConfigmap(ctx, c, namespace)
	if err != nil {
		t.Fatalf("Failed to delete CA configmap: (%v)", err)
	}
	err = openshift.DeleteCAConfigmap(ctx, c, namespace)
	if err != nil {
		t.Fatalf("Run into error when try to delete CA configmap twice: (%v)", err)
	}
}

func TestCreateDeleteMonitoringClusterRoleBinding(t *testing.T) {
	ctx := context.TODO()
	c := fake.NewClientBuilder().Build()
	namespace := "test-ns"
	saName := "test-sa"
	err := openshift.CreateMonitoringClusterRoleBinding(ctx, logr.Logger{}, c, namespace, saName)
	if err != nil {
		t.Fatalf("Failed to create clusterrolebinding: (%v)", err)
	}
	rb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: openshift.ClusterRoleBindingName,
			Annotations: map[string]string{
				openshift.OwnerLabelKey: openshift.OwnerLabelValue,
			},
			ResourceVersion: "1",
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "cluster-monitoring-view-test",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: namespace,
			},
		},
	}
	err = c.Update(context.TODO(), rb)
	if err != nil {
		t.Fatalf("Failed to update clusterrolebinding: (%v)", err)
	}
	err = openshift.CreateMonitoringClusterRoleBinding(ctx, logr.Logger{}, c, namespace, saName)
	if err != nil {
		t.Fatalf("Failed to revert clusterrolebinding: (%v)", err)
	}
	err = openshift.DeleteMonitoringClusterRoleBinding(ctx, c)
	if err != nil {
		t.Fatalf("Failed to delete clusterrolebinding: (%v)", err)
	}
	err = openshift.DeleteMonitoringClusterRoleBinding(ctx, c)
	if err != nil {
		t.Fatalf("Run into error when try to delete delete clusterrolebinding twice: (%v)", err)
	}
}

func TestGetClusterID(t *testing.T) {
	ctx := context.TODO()
	scheme := runtime.NewScheme()
	ocinfrav1.Install(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(cv).Build()
	found, err := openshift.GetClusterID(ctx, c)
	if err != nil {
		t.Fatalf("Failed to get clusterversion: (%v)", err)
	}
	if found != testClusterID {
		t.Fatalf("Got wrong cluster id" + found)
	}
}
