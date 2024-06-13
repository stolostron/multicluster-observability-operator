// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	observatoriumv1alpha1 "github.com/stolostron/observatorium-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
)

var (
	apiServerURL           = "http://example.com"
	clusterID              = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
	DefaultDSImgRepository = "quay.io:443/acm-d"
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
				AnnotationKeyImageTagSuffix:  "2.3.0-SNAPSHOT-2021-07-26-18-43-26",
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
				AnnotationKeyImageTagSuffix:  "2.3.0-SNAPSHOT-2021-07-26-18-43-26",
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
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(inf).Build()
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

func TestGetObsAPIRouteHost(t *testing.T) {
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
	scheme.AddKnownTypes(mcov1beta2.GroupVersion, &mcov1beta2.MultiClusterObservability{})
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(route).Build()

	host, err := GetObsAPIRouteHost(context.TODO(), client, "default")
	assert.NoError(t, err)
	if host == apiServerURL {
		t.Errorf("Should not get route host in default namespace")
	}

	host, err = GetObsAPIRouteHost(context.TODO(), client, "test")
	assert.NoError(t, err)
	if host != apiServerURL {
		t.Errorf("Observatorium api (%v) is not the expected (%v)", host, apiServerURL)
	}

	customBaseURL := "https://custom.base/url"
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name: GetMonitoringCRName(),
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			AdvancedConfig: &mcov1beta2.AdvancedConfig{
				CustomObservabilityHubURL: mcoshared.URL(customBaseURL),
			},
		},
	}
	client = fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(route, mco).Build()
	host, err = GetObsAPIRouteHost(context.TODO(), client, "test")
	assert.NoError(t, err)
	if host != apiServerURL {
		t.Errorf("Observatorium api (%v) is not the expected (%v)", host, apiServerURL)
	}

	mco.Spec.AdvancedConfig.CustomObservabilityHubURL = "httpa://foob ar.c"
	client = fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(route, mco).Build()
	host, err = GetObsAPIRouteHost(context.TODO(), client, "test")
	assert.NoError(t, err)
	if host != apiServerURL {
		t.Errorf("Observatorium api (%v) is not the expected (%v)", host, apiServerURL)
	}
}

func TestGetObsAPIExternalHost(t *testing.T) {
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
	scheme.AddKnownTypes(mcov1beta2.GroupVersion, &mcov1beta2.MultiClusterObservability{})
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(route).Build()

	obsAPIURL, err := GetObsAPIExternalURL(context.TODO(), client, "default")
	assert.NoError(t, err)
	if obsAPIURL.String() == apiServerURL {
		t.Errorf("Should not get route host in default namespace")
	}

	obsAPIURL, err = GetObsAPIExternalURL(context.TODO(), client, "test")
	assert.NoError(t, err)
	if obsAPIURL.String() != apiServerURL {
		t.Errorf("Observatorium api (%v) is not the expected (%v)", obsAPIURL, apiServerURL)
	}

	customBaseURL := "https://custom.base/url"
	expectedURL := "https://custom.base/url"
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name: GetMonitoringCRName(),
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			AdvancedConfig: &mcov1beta2.AdvancedConfig{
				CustomObservabilityHubURL: mcoshared.URL(customBaseURL),
			},
		},
	}
	client = fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(route, mco).Build()
	obsAPIURL, err = GetObsAPIExternalURL(context.TODO(), client, "test")
	assert.NoError(t, err)
	if obsAPIURL.String() != expectedURL {
		t.Errorf("Observatorium api (%v) is not the expected (%v)", obsAPIURL, expectedURL)
	}

	mco.Spec.AdvancedConfig.CustomObservabilityHubURL = "httpa://foob ar.c"
	client = fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(route, mco).Build()
	_, err = GetObsAPIExternalURL(context.TODO(), client, "test")
	if err == nil {
		t.Errorf("expected error when parsing URL '%v', but got none", mco.Spec.AdvancedConfig.CustomObservabilityHubURL)
	}
}

func TestGetAlertmanagerEndpoint(t *testing.T) {
	routeURL := "http://route.example.com"
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AlertmanagerRouteName,
			Namespace: "test",
		},
		Spec: routev1.RouteSpec{
			Host: routeURL,
		},
	}
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(routev1.GroupVersion, route)
	scheme.AddKnownTypes(mcov1beta2.GroupVersion, &mcov1beta2.MultiClusterObservability{})
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(route).Build()

	host, _ := GetAlertmanagerEndpoint(context.TODO(), client, "default")
	if host == routeURL {
		t.Errorf("Should not get route host in default namespace")
	}

	host, _ = GetAlertmanagerEndpoint(context.TODO(), client, "test")
	if host != routeURL {
		t.Errorf("Alertmanager URL (%v) is not the expected (%v)", host, routeURL)
	}

	customBaseURL := "https://custom.base/url"
	mco := &mcov1beta2.MultiClusterObservability{
		ObjectMeta: metav1.ObjectMeta{
			Name: GetMonitoringCRName(),
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			AdvancedConfig: &mcov1beta2.AdvancedConfig{
				CustomAlertmanagerHubURL: mcoshared.URL(customBaseURL),
			},
		},
	}
	client = fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(route, mco).Build()
	host, _ = GetAlertmanagerEndpoint(context.TODO(), client, "test")
	if host != customBaseURL {
		t.Errorf("Alertmanager URL (%v) is not the expected (%v)", host, customBaseURL)
	}

	mco.Spec.AdvancedConfig.CustomAlertmanagerHubURL = "httpa://foob ar.c"
	client = fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(route, mco).Build()
	_, err := GetAlertmanagerEndpoint(context.TODO(), client, "test")
	if err == nil {
		t.Errorf("expected error when parsing URL '%v', but got none", mco.Spec.AdvancedConfig.CustomObservabilityHubURL)
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
	s := runtime.NewScheme()
	s.AddKnownTypes(mcov1beta2.GroupVersion, mco)
	s.AddKnownTypes(observatoriumv1alpha1.GroupVersion, obs)
	objs := []runtime.Object{mco, obs}
	return fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).Build()
}

func TestReadImageManifestConfigMap(t *testing.T) {
	buildTestImageManifestCM := func(ns, version string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ImageManifestConfigMapNamePrefix + version,
				Namespace: ns,
				Labels: map[string]string{
					OCMManifestConfigMapTypeLabelKey:    OCMManifestConfigMapTypeLabelValue,
					OCMManifestConfigMapVersionLabelKey: version,
				},
			},
			Data: map[string]string{
				"test-key": fmt.Sprintf("test-value:%s", version),
			},
		}
	}

	ns := "testing"
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)

	caseList := []struct {
		name         string
		inputCMList  []string
		version      string
		expectedData map[string]string
		expectedRet  bool
		preFunc      func()
	}{
		{
			name:         "no image manifest configmap",
			inputCMList:  []string{},
			version:      "2.3.0",
			expectedRet:  false,
			expectedData: map[string]string{},
			preFunc: func() {
				os.Setenv("POD_NAMESPACE", ns)
				SetImageManifests(map[string]string{})
			},
		},
		{
			name:         "single valid image manifest configmap",
			inputCMList:  []string{"2.2.3"},
			version:      "2.3.0",
			expectedRet:  false,
			expectedData: map[string]string{},
			preFunc: func() {
				os.Setenv("POD_NAMESPACE", ns)
				SetImageManifests(map[string]string{})
			},
		},
		{
			name:        "multiple valid image manifest configmaps",
			inputCMList: []string{"2.2.3", "2.3.0"},
			version:     "2.3.0",
			expectedRet: true,
			expectedData: map[string]string{
				"test-key": "test-value:2.3.0",
			},
			preFunc: func() {
				os.Setenv("POD_NAMESPACE", ns)
				SetImageManifests(map[string]string{})
			},
		},
		{
			name:        "multiple image manifest configmaps with invalid",
			inputCMList: []string{"2.2.3", "2.3.0", "invalid"},
			version:     "2.3.0",
			expectedRet: true,
			expectedData: map[string]string{
				"test-key": "test-value:2.3.0",
			},
			preFunc: func() {
				os.Setenv("POD_NAMESPACE", ns)
				SetImageManifests(map[string]string{})
			},
		},
		{
			name:         "valid image manifest configmaps with no namespace set",
			inputCMList:  []string{"2.2.3", "2.3.0"},
			version:      "2.3.0",
			expectedRet:  false,
			expectedData: map[string]string{},
			preFunc: func() {
				os.Unsetenv("POD_NAMESPACE")
				SetImageManifests(map[string]string{})
			},
		},
	}

	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			c.preFunc()
			initObjs := []runtime.Object{}
			for _, cmName := range c.inputCMList {
				initObjs = append(initObjs, buildTestImageManifestCM(ns, cmName))
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(initObjs...).Build()

			gotRet, err := ReadImageManifestConfigMap(client, c.version)
			if err != nil {
				t.Errorf("Failed read image manifest configmap due to %v", err)
			}
			if gotRet != c.expectedRet {
				t.Errorf("case (%v) output (%v) is not the expected (%v)", c.name, gotRet, c.expectedRet)
			}
			if !reflect.DeepEqual(GetImageManifests(), c.expectedData) {
				t.Errorf("case (%v) output (%v) is not the expected (%v)", c.name, GetImageManifests(), c.expectedData)
			}
		})
	}
}

func Test_checkIsIBMCloud(t *testing.T) {
	s := runtime.NewScheme()
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Node{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.NodeList{})
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
				client: fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(nodeOther).Build(),
				name:   "test-secret",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "is ibm",
			args: args{
				client: fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(nodeIBM).Build(),
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
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
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
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
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
				RBACQueryProxy: &mcov1beta2.CommonSpec{
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
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
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
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
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
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
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
				ObservatoriumAPI: &mcov1beta2.CommonSpec{},
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
		{
			name:          "No advanced defined",
			componentName: Grafana,
			raw:           nil,
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == GrafanaCPURequets &&
					resources.Requests.Memory().String() == GrafanaMemoryRequets &&
					resources.Limits.Cpu().String() == GrafanaCPULimits &&
					resources.Limits.Memory().String() == GrafanaMemoryLimits
			},
		},
		{
			name:          "Have requests defined",
			componentName: Grafana,
			raw: &mcov1beta2.AdvancedConfig{
				Grafana: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == "1" &&
					resources.Requests.Memory().String() == GrafanaMemoryRequets &&
					resources.Limits.Cpu().String() == GrafanaCPULimits &&
					resources.Limits.Memory().String() == GrafanaMemoryLimits
			},
		},
		{
			name:          "Have limits defined",
			componentName: Grafana,
			raw: &mcov1beta2.AdvancedConfig{
				Grafana: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == GrafanaCPURequets &&
					resources.Requests.Memory().String() == GrafanaMemoryRequets &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == GrafanaMemoryLimits
			},
		},
		{
			name:          "Have limits defined",
			componentName: Grafana,
			raw: &mcov1beta2.AdvancedConfig{
				Grafana: &mcov1beta2.CommonSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == GrafanaCPURequets &&
					resources.Requests.Memory().String() == GrafanaMemoryRequets &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "1Gi"
			},
		},
		{
			name:          "Have limits defined",
			componentName: ThanosQueryFrontendMemcached,
			raw: &mcov1beta2.AdvancedConfig{
				QueryFrontendMemcached: &mcov1beta2.CacheConfig{
					CommonSpec: mcov1beta2.CommonSpec{
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == ThanosCachedCPURequets &&
					resources.Requests.Memory().String() == ThanosCachedMemoryRequets &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "1Gi"
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
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
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
				ObservatoriumAPI: &mcov1beta2.CommonSpec{
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
				ObservatoriumAPI: &mcov1beta2.CommonSpec{},
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

func TestGetOBAResources(t *testing.T) {
	caseList := []struct {
		name          string
		componentName string
		raw           *mcoshared.ObservabilityAddonSpec
		result        func(resources corev1.ResourceRequirements) bool
	}{
		{
			name:          "Have requests defined",
			componentName: ObservatoriumAPI,
			raw: &mcoshared.ObservabilityAddonSpec{
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
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
			name:          "Have limits defined",
			componentName: ObservatoriumAPI,
			raw: &mcoshared.ObservabilityAddonSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1"),
					},
				},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == MetricsCollectorCPURequets &&
					resources.Requests.Memory().String() == MetricsCollectorMemoryRequets &&
					resources.Limits.Cpu().String() == "1" &&
					resources.Limits.Memory().String() == "0"
			},
		},
		{
			name:          "no resources defined",
			componentName: ObservatoriumAPI,
			raw: &mcoshared.ObservabilityAddonSpec{
				Resources: &corev1.ResourceRequirements{},
			},
			result: func(resources corev1.ResourceRequirements) bool {
				return resources.Requests.Cpu().String() == MetricsCollectorCPURequets &&
					resources.Requests.Memory().String() == MetricsCollectorMemoryRequets &&
					resources.Limits.Cpu().String() == "0" &&
					resources.Limits.Memory().String() == "0"
			},
		},
	}
	for _, c := range caseList {
		t.Run(c.componentName+":"+c.name, func(t *testing.T) {
			resources := GetOBAResources(c.raw)
			if !c.result(*resources) {
				t.Errorf("case (%v) output (%v) is not the expected", c.componentName+":"+c.name, resources)
			}
		})
	}
}

func TestGetOperandName(t *testing.T) {
	caseList := []struct {
		name          string
		componentName string
		prepare       func()
		result        func() bool
	}{
		{
			name:          "No Observatorium CR",
			componentName: Alertmanager,
			prepare: func() {
				SetOperandNames(fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build())
			},
			result: func() bool {
				return GetOperandName(Alertmanager) == GetOperandNamePrefix()+"alertmanager"
			},
		},
		{
			name:          "Have Observatorium CR without ownerreference",
			componentName: Alertmanager,
			prepare: func() {
				// clean the operandNames map
				CleanUpOperandNames()
				mco := &mcov1beta2.MultiClusterObservability{
					TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
					ObjectMeta: metav1.ObjectMeta{
						Name: GetDefaultCRName(),
					},
					Spec: mcov1beta2.MultiClusterObservabilitySpec{
						StorageConfig: &mcov1beta2.StorageConfig{
							MetricObjectStorage: &mcoshared.PreConfiguredStorage{
								Key:  "test",
								Name: "test",
							},
						},
					},
				}

				observatorium := &observatoriumv1alpha1.Observatorium{
					ObjectMeta: metav1.ObjectMeta{
						Name:      GetOperandNamePrefix() + "-observatorium",
						Namespace: GetDefaultNamespace(),
					},
				}

				// Register operator types with the runtime scheme.
				s := runtime.NewScheme()
				mcov1beta2.SchemeBuilder.AddToScheme(s)
				observatoriumv1alpha1.AddToScheme(s)
				client := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(mco, observatorium).Build()
				SetMonitoringCRName(GetDefaultCRName())
				SetOperandNames(client)
			},
			result: func() bool {
				return GetOperandName(Alertmanager) == GetOperandNamePrefix()+Alertmanager &&
					GetOperandName(Grafana) == GetOperandNamePrefix()+Grafana &&
					GetOperandName(Observatorium) == GetDefaultCRName()
			},
		},
		{
			name:          "Have Observatorium CR (observability-observatorium) with ownerreference",
			componentName: Alertmanager,
			prepare: func() {
				// clean the operandNames map
				CleanUpOperandNames()
				mco := &mcov1beta2.MultiClusterObservability{
					TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
					ObjectMeta: metav1.ObjectMeta{
						Name: GetDefaultCRName(),
					},
					Spec: mcov1beta2.MultiClusterObservabilitySpec{
						StorageConfig: &mcov1beta2.StorageConfig{
							MetricObjectStorage: &mcoshared.PreConfiguredStorage{
								Key:  "test",
								Name: "test",
							},
						},
					},
				}

				observatorium := &observatoriumv1alpha1.Observatorium{
					ObjectMeta: metav1.ObjectMeta{
						Name:      GetOperandNamePrefix() + "observatorium",
						Namespace: GetDefaultNamespace(),
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "MultiClusterObservability",
								Name: GetDefaultCRName(),
							},
						},
					},
				}

				// Register operator types with the runtime scheme.
				s := runtime.NewScheme()
				mcov1beta2.SchemeBuilder.AddToScheme(s)
				observatoriumv1alpha1.AddToScheme(s)
				client := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(mco, observatorium).Build()

				SetMonitoringCRName(GetDefaultCRName())
				SetOperandNames(client)
			},
			result: func() bool {
				return GetOperandName(Alertmanager) == Alertmanager &&
					GetOperandName(Grafana) == Grafana &&
					GetOperandName(Observatorium) == GetOperandNamePrefix()+"observatorium"
			},
		},
		{
			name:          "Have Observatorium CR (observability) with ownerreference",
			componentName: Alertmanager,
			prepare: func() {
				// clean the operandNames map
				CleanUpOperandNames()
				mco := &mcov1beta2.MultiClusterObservability{
					TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
					ObjectMeta: metav1.ObjectMeta{
						Name: GetDefaultCRName(),
					},
					Spec: mcov1beta2.MultiClusterObservabilitySpec{
						StorageConfig: &mcov1beta2.StorageConfig{
							MetricObjectStorage: &mcoshared.PreConfiguredStorage{
								Key:  "test",
								Name: "test",
							},
						},
					},
				}

				observatorium := &observatoriumv1alpha1.Observatorium{
					ObjectMeta: metav1.ObjectMeta{
						Name:      GetDefaultCRName(),
						Namespace: GetDefaultNamespace(),
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind: "MultiClusterObservability",
								Name: GetDefaultCRName(),
							},
						},
					},
				}

				// Register operator types with the runtime scheme.
				s := runtime.NewScheme()
				mcov1beta2.SchemeBuilder.AddToScheme(s)
				observatoriumv1alpha1.AddToScheme(s)
				client := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(mco, observatorium).Build()

				SetMonitoringCRName(GetDefaultCRName())
				SetOperandNames(client)
			},
			result: func() bool {
				return GetOperandName(Alertmanager) == GetOperandNamePrefix()+Alertmanager &&
					GetOperandName(Grafana) == GetOperandNamePrefix()+Grafana &&
					GetOperandName(Observatorium) == GetDefaultCRName()
			},
		},
	}
	for _, c := range caseList {
		t.Run(c.name, func(t *testing.T) {
			c.prepare()
			if !c.result() {
				t.Errorf("case (%v) output is not the expected", c.name)
			}
		})
	}
}
