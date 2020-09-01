// Copyright (c) 2020 Red Hat, Inc.

package config

import (
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	apiServerURL = "http://example.com"
	clusterID    = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
)

func TestGetClusterNameLabelKey(t *testing.T) {
	clusterName := GetClusterNameLabelKey()
	if clusterName != clusterNameLabelKey {
		t.Errorf("Cluster Label (%v) is not the expected (%v)", clusterName, clusterNameLabelKey)
	}
}

func TestIsNeededReplacement(t *testing.T) {
	caseList := []struct {
		annotations map[string]string
		name        string
		imageRepo   string
		expected    bool
	}{
		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultImgRepository,
				AnnotationKeyImageTagSuffix:  "test",
			},
			name:      "Image is in different org",
			imageRepo: "test.org",
			expected:  false,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultImgRepository,
				AnnotationKeyImageTagSuffix:  "2.1.0-SNAPSHOT-2020-08-11-14-16-48",
			},
			name:      "Image is in the same org",
			imageRepo: DefaultImgRepository,
			expected:  true,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultDSImgRepository,
				AnnotationKeyImageTagSuffix:  "2.1.0-SNAPSHOT-2020-08-11-14-16-48",
			},
			name:      "Image is from the ds build",
			imageRepo: "test.org",
			expected:  true,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: "test.org",
			},
			name:      "no img tag",
			imageRepo: "test.org",
			expected:  false,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageTagSuffix: "test",
			},
			name:      "no img repo",
			imageRepo: "test.org",
			expected:  false,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: "",
				AnnotationKeyImageTagSuffix:  "",
			},
			name:      "the img repo is empty",
			imageRepo: "",
			expected:  false,
		},

		{
			annotations: map[string]string{},
			name:        "no img info",
			imageRepo:   "test.org",
			expected:    false,
		},

		{
			annotations: nil,
			name:        "annotations is nil",
			imageRepo:   "test.org",
			expected:    false,
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			output := IsNeededReplacement(c.annotations, c.imageRepo)
			if output != c.expected {
				t.Errorf("case (%v) output (%v) is not the expected (%v)", c.name, output, c.expected)
			}
		})
	}
}

func TestGetDefaultTenantName(t *testing.T) {
	tenantName := GetDefaultTenantName()
	if tenantName != defaultTenantName {
		t.Errorf("Tenant name (%v) is not the expected (%v)", tenantName, defaultTenantName)
	}
}

func TestGetDefaultNamespace(t *testing.T) {
	expected := "open-cluster-management-observability"
	if GetDefaultNamespace() != expected {
		t.Errorf("Default Namespace (%v) is not the expected (%v)", GetDefaultNamespace(), expected)
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
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(configv1.GroupVersion, inf)
	client := fake.NewFakeClientWithScheme(scheme, inf)
	apiURL, _ := GetKubeAPIServerAddress(client)
	if apiURL != apiServerURL {
		t.Errorf("Kubenetes API Server Address (%v) is not the expected (%v)", apiURL, apiServerURL)
	}
}

func TestGetClusterIDSuccess(t *testing.T) {
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

func TestGetClusterIDFailed(t *testing.T) {
	inf := &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{Name: infrastructureConfigName},
		Status: configv1.InfrastructureStatus{
			APIServerURL: apiServerURL,
		},
	}
	client := fakeconfigclient.NewSimpleClientset(inf)
	_, err := GetClusterID(client)
	if err == nil {
		t.Errorf("Should throw the error since there is no clusterversion defined")
	}
}

func TestGetObsAPIUrl(t *testing.T) {
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAPIGateway,
			Namespace: "test",
		},
		Spec: routev1.RouteSpec{
			Host: apiServerURL,
		},
	}
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(routev1.GroupVersion, route)
	client := fake.NewFakeClientWithScheme(scheme, route)

	host, _ := GetObsAPIUrl(client, "default")
	if host == apiServerURL {
		t.Errorf("Should not get route host in default namespace")
	}
	host, _ = GetObsAPIUrl(client, "test")
	if host != apiServerURL {
		t.Errorf("Observatorium api (%v) is not the expected (%v)", host, apiServerURL)
	}
}

func TestGetAnnotationImageInfo(t *testing.T) {
	SetAnnotationImageInfo(map[string]string{
		AnnotationKeyImageRepository: DefaultImgRepository,
		AnnotationKeyImageTagSuffix:  DefaultImgTagSuffix,
	})
	imageInfo := GetAnnotationImageInfo()

	if imageInfo.ImageRepository != DefaultImgRepository {
		t.Errorf("ImageRepository (%v) is not the expected (%v)", imageInfo.ImageRepository, DefaultImgRepository)
	}
	if imageInfo.ImageTagSuffix != DefaultImgTagSuffix {
		t.Errorf("ImageTagSuffix (%v) is not the expected (%v)", imageInfo.ImageRepository, DefaultImgRepository)
	}
}
