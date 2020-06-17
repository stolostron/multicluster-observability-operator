// Copyright (c) 2020 Red Hat, Inc.

package config

import (
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	apiServerURL = "http://example.com"
	clusterID    = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
)

func newFakeClient(gvs []schema.GroupVersion, types []runtime.Object) client.Client {
	s := scheme.Scheme
	for k, gv := range gvs {
		s.AddKnownTypes(gv, types[k])
	}
	return fake.NewFakeClientWithScheme(s, types...)
}

func TestGetClusterNameLabelKey(t *testing.T) {
	clusterName := GetClusterNameLabelKey()
	if clusterName != clusterNameLabelKey {
		t.Errorf("Cluster Label (%v) is not the expected (%v)", clusterName, clusterNameLabelKey)
	}
}

func TestMonitoringCRName(t *testing.T) {
	var monitoringCR = "monitoring"
	SetMonitoringCRName(monitoringCR)

	if monitoringCR != GetMonitoringCRName() {
		t.Errorf("Monitoring CR Name (%v) is not the expected (%v)", GetMonitoringCRName(), monitoringCR)
	}
}

func TestGetKubeAPIServerAddress(t *testing.T) {
	inf := &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{Name: infrastructureConfigName},
		Status: configv1.InfrastructureStatus{
			APIServerURL: apiServerURL,
		},
	}
	client := newFakeClient([]schema.GroupVersion{configv1.GroupVersion}, []runtime.Object{inf})
	apiURL, _ := GetKubeAPIServerAddress(client)
	if apiURL != apiServerURL {
		t.Errorf("Kubenetes API Server Address (%v) is not the expected (%v)", apiURL, apiServerURL)
	}
}

func TestGetClusterID(t *testing.T) {
	version := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: configv1.ClusterID(clusterID),
		},
	}
	client := fakeconfigclient.NewSimpleClientset(version)
	tmpClusterID, _ := GetClusterID(client)
	if tmpClusterID != clusterID {
		t.Errorf("OCP ClusterID (%v) is not the expected (%v)", tmpClusterID, clusterID)
	}
}
