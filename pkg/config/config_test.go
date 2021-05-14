// Copyright (c) 2021 Red Hat, Inc.

package config

import (
	"os"
	"reflect"
	"testing"

	observatoriumv1alpha1 "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
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
	version      = "2.1.1"
)

func TestGetClusterNameLabelKey(t *testing.T) {
	clusterName := GetClusterNameLabelKey()
	if clusterName != clusterNameLabelKey {
		t.Errorf("Cluster Label (%v) is not the expected (%v)", clusterName, clusterNameLabelKey)
	}
}

func TestReplaceImage(t *testing.T) {

	caseList := []struct {
		annotations map[string]string
		name        string
		imageRepo   string
		expected    bool
		cm          map[string]string
	}{
		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultImgRepository,
				"mco-test-tag":               "test",
			},
			name:      "Replace image for test purpose",
			imageRepo: "test.org",
			expected:  true,
			cm:        nil,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultImgRepository,
				AnnotationKeyImageTagSuffix:  "test",
			},
			name:      "Image is in different org",
			imageRepo: "test.org",
			expected:  false,
			cm:        nil,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultImgRepository,
			},
			name:      "Image is in different org",
			imageRepo: "test.org",
			expected:  false,
			cm:        nil,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultImgRepository,
				AnnotationKeyImageTagSuffix:  "2.1.0-SNAPSHOT-2020-08-11-14-16-48",
			},
			name:      "Image is in the same org",
			imageRepo: DefaultImgRepository,
			expected:  true,
			cm:        nil,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultImgRepository,
			},
			name:      "Image is in the same org",
			imageRepo: DefaultImgRepository,
			expected:  false,
			cm:        nil,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultImgRepository,
			},
			name:      "Image is in the same org",
			imageRepo: DefaultImgRepository,
			expected:  true,
			cm: map[string]string{
				"test": "test.org",
			},
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultDSImgRepository,
				AnnotationKeyImageTagSuffix:  "2.1.0-SNAPSHOT-2020-08-11-14-16-48",
			},
			name:      "Image is from the ds build",
			imageRepo: "test.org",
			expected:  false,
			cm:        nil,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultDSImgRepository,
			},
			name:      "Image is from the ds build",
			imageRepo: "test.org",
			expected:  true,
			cm: map[string]string{
				"test": "test.org",
			},
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: DefaultDSImgRepository,
			},
			name:      "Image is from the ds build",
			imageRepo: "test.org",
			expected:  false,
			cm:        nil,
		},

		{
			annotations: map[string]string{
				AnnotationKeyImageRepository: "",
				AnnotationKeyImageTagSuffix:  "",
			},
			name:      "the img repo is empty",
			imageRepo: "",
			expected:  false,
			cm:        nil,
		},

		{
			annotations: map[string]string{},
			name:        "no img info",
			imageRepo:   "test.org",
			expected:    false,
			cm:          nil,
		},

		{
			annotations: nil,
			name:        "annotations is nil",
			imageRepo:   "test.org",
			expected:    false,
			cm:          nil,
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			SetImageManifests(c.cm)
			output, _ := ReplaceImage(c.annotations, c.imageRepo, "test")
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
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Annotations: map[string]string{
				AnnotationKeyImageTagSuffix: "tag",
			},
		},
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

	if len(mco.Spec.Tolerations) != 0 {
		t.Errorf("Tolerations (%v) is not the expected ([])", mco.Spec.Tolerations)
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
		Interval:      30,
	}

	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
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

	if len(mco.Spec.Tolerations) != 0 {
		t.Errorf("Tolerations (%v) is not the expected ([])", mco.Spec.Tolerations)
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

func TestReadImageManifestConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ImageManifestConfigMapName + version,
			Namespace: "ns2",
		},
		Data: map[string]string{
			"test-key": "test-value-1",
		},
	}
	os.Setenv("POD_NAMESPACE", "ns2")
	os.Setenv("COMPONENT_VERSION", version)
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	client := fake.NewFakeClientWithScheme(scheme, cm)

	caseList := []struct {
		expected bool
		name     string
		data     map[string]string
		preFunc  func()
	}{
		{
			name:     "read the " + ImageManifestConfigMapName + "2.1.1",
			expected: true,
			data: map[string]string{
				"test-key": "test-value-1",
			},
			preFunc: func() {},
		},
		{
			name:     "Should not read the " + ImageManifestConfigMapName + "2.1.1 again",
			expected: false,
			data: map[string]string{
				"test-key": "test-value-1",
			},
			preFunc: func() {},
		},
		{
			name:     ImageManifestConfigMapName + "2.1.1 configmap does not exist",
			expected: true,
			data:     map[string]string{},
			preFunc: func() {
				SetImageManifests(map[string]string{})
				os.Setenv(ComponentVersion, "invalid")
			},
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			c.preFunc()
			output, err := ReadImageManifestConfigMap(client)
			if err != nil {
				t.Errorf("Failed read image manifest configmap due to %v", err)
			}
			if output != c.expected {
				t.Errorf("case (%v) output (%v) is not the expected (%v)", c.name, output, c.expected)
			}
			if !reflect.DeepEqual(GetImageManifests(), c.data) {
				t.Errorf("case (%v) output (%v) is not the expected (%v)", c.name, GetImageManifests(), c.data)
			}
		})
	}
}

func Test_checkIsIBMCloud(t *testing.T) {
	s := scheme.Scheme
	nodeIBM := &corev1.Node{
		Spec: corev1.NodeSpec{
			ProviderID: "ibm",
		},
	}
	nodeOther := &corev1.Node{}

	type args struct {
		client client.Client
		name   string
	}
	caselist := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "is normal ocp",
			args: args{
				client: fake.NewFakeClientWithScheme(s, []runtime.Object{nodeOther}...),
				name:   "test-secret",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "is ibm",
			args: args{
				client: fake.NewFakeClientWithScheme(s, []runtime.Object{nodeIBM}...),
				name:   "test-secret",
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, c := range caselist {
		t.Run(c.name, func(t *testing.T) {
			got, err := CheckIsIBMCloud(c.args.client)
			if (err != nil) != c.wantErr {
				t.Errorf("checkIsIBMCloud() error = %v, wantErr %v", err, c.wantErr)
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("checkIsIBMCloud() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestGetObjectPrefix(t *testing.T) {
	got := GetObjectPrefix()
	if got != "observability" {
		t.Errorf("GetObjectPrefix() = %v, want observability", got)
	}
}
