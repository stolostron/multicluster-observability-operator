// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

func TestUpdateHubClusterMonitoringConfig(t *testing.T) {

	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAPIGateway,
			Namespace: "test",
		},
		Spec: routev1.RouteSpec{
			Host: "apiServerURL",
		},
	}
	client := util.NewFakeClient([]schema.GroupVersion{routev1.GroupVersion}, []runtime.Object{route})

	version := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: configv1.ClusterID("xxxx-xxxx"),
		},
	}
	ocpClient := fakeconfigclient.NewSimpleClientset(version)

	_, err := UpdateHubClusterMonitoringConfig(client, ocpClient, "test")
	if err != nil {
		t.Errorf("Update configmap has error: %v", err)
	}

	cm, _ := getConfigMap(client)
	if cm.Data == nil {
		t.Errorf("Update configmap is failed")
	}
}
