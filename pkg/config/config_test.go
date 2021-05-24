// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
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
				"mco-test-image":             "test.org/test:latest",
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

func NewFakeClient(mco *mcov1beta2.MultiClusterObservability,
	obs *observatoriumv1alpha1.Observatorium) client.Client {
	s := scheme.Scheme
	s.AddKnownTypes(mcov1beta2.GroupVersion, mco)
	s.AddKnownTypes(observatoriumv1alpha1.GroupVersion, obs)
	objs := []runtime.Object{mco, obs}
	return fake.NewFakeClientWithScheme(s, objs...)
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

func TestGetResources(t *testing.T) {
	caseList := []struct {
		name          string
		componentName string
		raw           *mcov1beta2.AdvancedConfig
		result        func(resources corev1.ResourceRequirements) bool
	}{
		{
			name:          "Have requests defined in resources",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.ObservatoriumAPISpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == "1" &&
					resources.Requests.Memory().String() == "1Gi" &&
					resources.Limits.Cpu().String() == "0" &&
					resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "Have limits defined in resources",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.ObservatoriumAPISpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ObservatoriumAPICPURequets &&
					resources.Requests.Memory().String() == ObservatoriumAPIMemoryRequets &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "1Gi"
			},
		},
		{
			name:          "Have limits defined in resources",
			componentName: RBACQueryProxy,
			raw: &mcov1beta2.AdvancedConfig{
				RBACQueryProxy: &mcov1beta2.RBACQueryProxySpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == RBACQueryProxyCPURequets &&
					resources.Requests.Memory().String() == RBACQueryProxyMemoryRequets &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "1Gi"
			},
		},
		{
			name:          "Have requests and limits defined in requests",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.ObservatoriumAPISpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == "1" &&
					resources.Requests.Memory().String() == "1Gi" &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "1Gi"
			},
		},
		{
			name:          "No CPU defined in requests",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.ObservatoriumAPISpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ObservatoriumAPICPURequets &&
					resources.Requests.Memory().String() == ObservatoriumAPIMemoryRequets &&
					resources.Limits.Cpu().String() == "0" && resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "No requests defined in resources",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.ObservatoriumAPISpec{
					Resources: &corev1.ResourceRequirements{},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ObservatoriumAPICPURequets &&
					resources.Requests.Memory().String() == ObservatoriumAPIMemoryRequets &&
					resources.Limits.Cpu().String() == "0" && resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "No resources defined",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.ObservatoriumAPISpec{},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ObservatoriumAPICPURequets &&
					resources.Requests.Memory().String() == ObservatoriumAPIMemoryRequets &&
					resources.Limits.Cpu().String() == "0" && resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "No advanced defined",
			componentName: ObservatoriumAPI,
			raw:           nil,
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ObservatoriumAPICPURequets &&
					resources.Requests.Memory().String() == ObservatoriumAPIMemoryRequets &&
					resources.Limits.Cpu().String() == "0" && resources.Limits.Memory().String() == "0"
			},
		},
	}

	for _, c := range caseList {
		t.Run(c.componentName+":"+c.name, func(t *testing.T) {
			resources := GetResources(c.componentName, c.raw)
			if !c.result(resources) {
				t.Errorf("case (%v) output (%v) is not the expected", c.componentName+":"+c.name, resources)
			}
		})
	}
}

func TestGetReplicas(t *testing.T) {
	var replicas0 int32 = 0
	caseList := []struct {
		name          string
		componentName string
		raw           *mcov1beta2.AdvancedConfig
		result        func(replicas *int32) bool
	}{
		{
			name:          "Have replicas defined",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.ObservatoriumAPISpec{
					Replicas: &Replicas1,
				},
			},
			result: func(replicas *int32) bool {
				return replicas == &Replicas1
			},
		},
		{
			name:          "Do not allow to set 0",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.ObservatoriumAPISpec{
					Replicas: &replicas0,
				},
			},
			result: func(replicas *int32) bool {
				return replicas == &Replicas2
			},
		},
		{
			name:          "No advanced defined",
			componentName: ObservatoriumAPI,
			raw:           nil,
			result: func(replicas *int32) bool {
				return replicas == &Replicas2
			},
		},
		{
			name:          "No replicas defined",
			componentName: ObservatoriumAPI,
			raw: &mcov1beta2.AdvancedConfig{
				ObservatoriumAPI: &mcov1beta2.ObservatoriumAPISpec{},
			},
			result: func(replicas *int32) bool {
				return replicas == &Replicas2
			},
		},
	}
	for _, c := range caseList {
		t.Run(c.componentName+":"+c.name, func(t *testing.T) {
			replicas := GetReplicas(c.componentName, c.raw)
			if !c.result(replicas) {
				t.Errorf("case (%v) output (%v) is not the expected", c.componentName+":"+c.name, replicas)
			}
		})
	}
}
