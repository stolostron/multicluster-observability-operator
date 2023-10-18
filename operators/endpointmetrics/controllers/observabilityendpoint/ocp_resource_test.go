// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"fmt"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	infra = &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Status: ocinfrav1.InfrastructureStatus{
			ControlPlaneTopology: ocinfrav1.SingleReplicaTopologyMode,
		},
	}
	hCluster = &hyperv1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hostedClusterName,
			Namespace: hosteClusterNamespace,
		},
		Spec: hyperv1.HostedClusterSpec{
			ClusterID: "test-hosted-cluster-id",
		},
	}
	sm = &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdServiceMonitor,
			Namespace: fmt.Sprintf("%s-%s", hosteClusterNamespace, hostedClusterName),
		},
		Spec: promv1.ServiceMonitorSpec{},
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
	err := createCAConfigmap(ctx, c)
	if err != nil {
		t.Fatalf("Failed to create CA configmap: (%v)", err)
	}
	err = deleteCAConfigmap(ctx, c)
	if err != nil {
		t.Fatalf("Failed to delete CA configmap: (%v)", err)
	}
	err = deleteCAConfigmap(ctx, c)
	if err != nil {
		t.Fatalf("Run into error when try to delete CA configmap twice: (%v)", err)
	}
}

func TestCreateDeleteMonitoringClusterRoleBinding(t *testing.T) {
	ctx := context.TODO()
	c := fake.NewClientBuilder().Build()
	err := createMonitoringClusterRoleBinding(ctx, c)
	if err != nil {
		t.Fatalf("Failed to create clusterrolebinding: (%v)", err)
	}
	rb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleBindingName,
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
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
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		},
	}
	err = c.Update(context.TODO(), rb)
	if err != nil {
		t.Fatalf("Failed to update clusterrolebinding: (%v)", err)
	}
	err = createMonitoringClusterRoleBinding(ctx, c)
	if err != nil {
		t.Fatalf("Failed to revert clusterrolebinding: (%v)", err)
	}
	err = deleteMonitoringClusterRoleBinding(ctx, c)
	if err != nil {
		t.Fatalf("Failed to delete clusterrolebinding: (%v)", err)
	}
	err = deleteMonitoringClusterRoleBinding(ctx, c)
	if err != nil {
		t.Fatalf("Run into error when try to delete delete clusterrolebinding twice: (%v)", err)
	}
}

func TestGetClusterID(t *testing.T) {
	ctx := context.TODO()
	c := fake.NewClientBuilder().WithRuntimeObjects(cv).Build()
	found, err := getClusterID(ctx, c)
	if err != nil {
		t.Fatalf("Failed to get clusterversion: (%v)", err)
	}
	if found != testClusterID {
		t.Fatalf("Got wrong cluster id" + found)
	}
}

func TestServiceMonitors(t *testing.T) {
	ctx := context.TODO()
	c := fake.NewClientBuilder().WithRuntimeObjects(hCluster, sm).Build()
	err := createServiceMonitors(ctx, c)
	if err != nil {
		t.Fatalf("Failed to create ServiceMonitors: (%v)", err)
	}
	err = deleteServiceMonitors(ctx, c)
	if err != nil {
		t.Fatalf("Failed to delete ServiceMonitors: (%v)", err)
	}
}

func TestDeleteServiceMonitor(t *testing.T) {
	ctx := context.TODO()
	c := fake.NewClientBuilder().Build()
	err := deleteServiceMonitor(ctx, c, "test", "test")
	if err != nil {
		t.Fatalf("Failed to delete ServiceMonitors: (%v)", err)
	}
}
