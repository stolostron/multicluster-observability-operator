// Copyright (c) 2020 Red Hat, Inc.

package config

import (
	"reflect"
	"testing"

	observatoriumv1alpha1 "github.com/observatorium/deployments/operator/api/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
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

func TestIsPaused(t *testing.T) {
	caseList := []struct {
		annotations map[string]string
		expected    bool
		name        string
	}{
		{
			name: "without mco-pause",
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultImgRepository,
				AnnotationKeyImageTagSuffix:  "test",
			},
			expected: false,
		},
		{
			name: "mco-pause is empty",
			annotations: map[string]string{
				AnnotationMCOPause: "",
			},
			expected: false,
		},
		{
			name: "mco-pause is false",
			annotations: map[string]string{
				AnnotationMCOPause: "false",
			},
			expected: false,
		},
		{
			name: "mco-pause is true",
			annotations: map[string]string{
				AnnotationMCOPause: "true",
			},
			expected: true,
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			output := IsPaused(c.annotations)
			if output != c.expected {
				t.Errorf("case (%v) output (%v) is not the expected (%v)", c.name, output, c.expected)
			}
		})
	}
}

func NewFakeClient(mco *mcov1beta1.MultiClusterObservability,
	obs *observatoriumv1alpha1.Observatorium) client.Client {
	s := scheme.Scheme
	s.AddKnownTypes(mcov1beta1.SchemeGroupVersion, mco)
	s.AddKnownTypes(observatoriumv1alpha1.GroupVersion, obs)
	objs := []runtime.Object{mco, obs}
	return fake.NewFakeClientWithScheme(s, objs...)
}

func TestGenerateMonitoringCR(t *testing.T) {
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec: mcov1beta1.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta1.StorageConfigObject{},
		},
	}

	result, err := GenerateMonitoringCR(NewFakeClient(mco, &observatoriumv1alpha1.Observatorium{}), mco)
	if result != nil || err != nil {
		t.Errorf("Should return nil for result (%v) and err (%v)", result, err)
	}

	if mco.Spec.ImagePullPolicy != DefaultImgPullPolicy {
		t.Errorf("ImagePullPolicy (%v) is not the expected (%v)",
			mco.Spec.ImagePullPolicy, DefaultImgPullPolicy)
	}

	if mco.Spec.ImagePullSecret != DefaultImgPullSecret {
		t.Errorf("ImagePullSecret (%v) is not the expected (%v)",
			mco.Spec.ImagePullSecret, DefaultImgPullSecret)
	}

	if mco.Spec.NodeSelector == nil {
		t.Errorf("NodeSelector (%v) is not the expected (non-nil)", mco.Spec.NodeSelector)
	}

	if mco.Spec.StorageConfig.StatefulSetSize != DefaultStorageSize {
		t.Errorf("StatefulSetSize (%v) is not the expected (%v)",
			mco.Spec.StorageConfig.StatefulSetSize,
			DefaultStorageSize)
	}

	if mco.Spec.StorageConfig.StatefulSetStorageClass != DefaultStorageClass {
		t.Errorf("StatefulSetStorageClass (%v) is not the expected (%v)",
			mco.Spec.StorageConfig.StatefulSetStorageClass,
			DefaultStorageClass)
	}

	defaultObservabilityAddonSpec := &mcov1beta1.ObservabilityAddonSpec{
		EnableMetrics: true,
		Interval:      DefaultAddonInterval,
	}

	if !reflect.DeepEqual(mco.Spec.ObservabilityAddonSpec, defaultObservabilityAddonSpec) {
		t.Errorf("ObservabilityAddonSpec (%v) is not the expected (%v)",
			mco.Spec.ObservabilityAddonSpec,
			defaultObservabilityAddonSpec)
	}
}

func TestGenerateMonitoringCustomizedCR(t *testing.T) {
	addonSpec := &mcov1beta1.ObservabilityAddonSpec{
		EnableMetrics: true,
		Interval:      60,
	}

	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "test",
			Annotations: map[string]string{
				"test": "test",
			},
		},

		Spec: mcov1beta1.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta1.StorageConfigObject{
				StatefulSetSize:         "1Gi",
				StatefulSetStorageClass: "gp2",
			},
			ObservabilityAddonSpec: addonSpec,
		},
	}

	fakeClient := NewFakeClient(mco, &observatoriumv1alpha1.Observatorium{})
	result, err := GenerateMonitoringCR(fakeClient, mco)
	if result != nil || err != nil {
		t.Fatalf("Should return nil for result (%v) and err (%v)", result, err)
	}

	if mco.Spec.ImagePullPolicy != DefaultImgPullPolicy {
		t.Errorf("ImagePullPolicy (%v) is not the expected (%v)",
			mco.Spec.ImagePullPolicy, DefaultImgPullPolicy)
	}

	if mco.Spec.ImagePullSecret != DefaultImgPullSecret {
		t.Errorf("ImagePullSecret (%v) is not the expected (%v)",
			mco.Spec.ImagePullSecret, DefaultImgPullSecret)
	}

	if mco.Spec.NodeSelector == nil {
		t.Errorf("NodeSelector (%v) is not the expected (non-nil)", mco.Spec.NodeSelector)
	}

	if mco.Spec.StorageConfig.StatefulSetSize != "1Gi" {
		t.Errorf("StatefulSetSize (%v) is not the expected (%v)",
			mco.Spec.StorageConfig.StatefulSetSize, "1Gi")
	}

	if mco.Spec.StorageConfig.StatefulSetStorageClass != "gp2" {
		t.Errorf("StatefulSetStorageClass (%v) is not the expected (%v)",
			mco.Spec.StorageConfig.StatefulSetStorageClass, "gp2")
	}

	if !reflect.DeepEqual(mco.Spec.ObservabilityAddonSpec, addonSpec) {
		t.Errorf("ObservabilityAddonSpec (%v) is not the expected (%v)",
			mco.Spec.ObservabilityAddonSpec,
			addonSpec)
	}

	mco.Annotations[AnnotationKeyImageRepository] = "test_repo"
	mco.Annotations[AnnotationKeyImageTagSuffix] = "test_suffix"

	fakeClient = NewFakeClient(mco, &observatoriumv1alpha1.Observatorium{})
	result, err = GenerateMonitoringCR(fakeClient, mco)
	if result != nil || err != nil {
		t.Fatalf("Should return nil for result (%v) and err (%v)", result, err)
	}

	if util.GetAnnotation(mco.Annotations, AnnotationKeyImageTagSuffix) != "test_suffix" {
		t.Errorf("ImageTagSuffix (%v) is not the expected (%v)",
			util.GetAnnotation(mco.Annotations, AnnotationKeyImageTagSuffix), "test_suffix")
	}
}
