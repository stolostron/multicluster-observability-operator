// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAlertManagerRenderer(t *testing.T) {
	clientCa := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "extension-apiserver-authentication",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"client-ca-file": "test",
		},
	}

	containerNameToMchKey := map[string]string{
		"alertmanager":    "prometheus_alertmanager",
		"config-reloader": "configmap_reloader",
		"kube-rbac-proxy": "kube_rbac_proxy",
	}
	mchImageManifest := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mch-image-manifest",
			Namespace: config.GetMCONamespace(),
			Labels: map[string]string{
				config.OCMManifestConfigMapTypeLabelKey:    config.OCMManifestConfigMapTypeLabelValue,
				config.OCMManifestConfigMapVersionLabelKey: "v1",
			},
		},
		Data: map[string]string{
			"prometheus_alertmanager": "quay.io/rhacm2/alertmanager:latest",
			"configmap_reloader":      "quay.io/rhacm2/configmap-reloader:latest",
			"kube_rbac_proxy":         "quay.io/rhacm2/kube-rbac-proxy:latest",
		},
	}

	kubeClient := fake.NewClientBuilder().WithObjects(clientCa, mchImageManifest).Build()

	alertResources := renderTemplates(t, kubeClient, makeBaseMco())

	// clientCa configmap must be filled with the client-ca-file data
	clientCaData := getResource[*corev1.ConfigMap](alertResources, "alertmanager-clientca-metric")
	assert.Equal(t, clientCa.Data["client-ca-file"], clientCaData.Data["client-ca-file"])

	// container images must be replaced with the ones from the mch-image-manifest configmap
	for _, obj := range alertResources {
		if obj.GetKind() == "StatefulSet" { // there is only one statefulset
			sts := &appsv1.StatefulSet{}
			runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, sts)
			for _, container := range sts.Spec.Template.Spec.Containers {
				// oauth-proxy container is not in the mch-image-manifest configmap
				// we use image-streams to get image for oauth-proxy
				if container.Name == "alertmanager-proxy" {
					continue
				}
				assert.Equal(t, mchImageManifest.Data[containerNameToMchKey[container.Name]], container.Image)
			}
		}
	}

	// namespace must be set to the one provided in the arguments
	for _, obj := range alertResources {
		if obj.GetKind() == "ClusterRole" || obj.GetKind() == "ClusterRoleBinding" {
			continue
		}

		if obj.GetName() == "acm-observability-alert-rules" { // has no update annotation
			continue
		}

		assert.Equal(t, "namespace", obj.GetNamespace(), fmt.Sprintf("kind: %s, name: %s", obj.GetKind(), obj.GetName()))
	}

	// alertmanager-proxy must have the secret value generated
	proxy := getResource[*corev1.Secret](alertResources, "alertmanager-proxy")
	assert.True(t, len(proxy.Data["session_secret"]) > 0)

}

func TestAlertManagerRendererMCOConfig(t *testing.T) {
	testCases := map[string]struct {
		mco    func() *mcov1beta2.MultiClusterObservability
		expect func(*testing.T, *appsv1.StatefulSet)
	}{
		"storage": {
			mco: func() *mcov1beta2.MultiClusterObservability {
				ret := makeBaseMco()
				ret.Spec.StorageConfig.AlertmanagerStorageSize = "12Gi"
				ret.Spec.StorageConfig.StorageClass = "mystorage"
				return ret
			},
			expect: func(t *testing.T, sts *appsv1.StatefulSet) {
				qty := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
				assert.Equal(t, "12Gi", qty.String())
				assert.Equal(t, "mystorage", *sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName)
			},
		},
		"imagePullPolicy": {
			mco: func() *mcov1beta2.MultiClusterObservability {
				ret := makeBaseMco()
				ret.Spec.ImagePullPolicy = corev1.PullNever
				return ret
			},
			expect: func(t *testing.T, sts *appsv1.StatefulSet) {
				for _, container := range sts.Spec.Template.Spec.Containers {
					assert.Equal(t, corev1.PullNever, container.ImagePullPolicy)
				}
			},
		},
		"imagePullSecret": {
			mco: func() *mcov1beta2.MultiClusterObservability {
				ret := makeBaseMco()
				ret.Spec.ImagePullSecret = "mysecret"
				return ret
			},
			expect: func(t *testing.T, sts *appsv1.StatefulSet) {
				assert.Equal(t, "mysecret", sts.Spec.Template.Spec.ImagePullSecrets[0].Name)
			},
		},
		"more than one replicas": {
			mco: func() *mcov1beta2.MultiClusterObservability {
				ret := makeBaseMco()
				replicas := int32(3)
				ret.Spec.AdvancedConfig = &mcov1beta2.AdvancedConfig{
					Alertmanager: &mcov1beta2.CommonSpec{
						Replicas: &replicas,
					},
				}
				return ret
			},
			expect: func(t *testing.T, sts *appsv1.StatefulSet) {
				assert.Equal(t, int32(3), *sts.Spec.Replicas)
				// args must contain --cluster.peer flag with the correct number of replicas
				args := sts.Spec.Template.Spec.Containers[0].Args
				count := 0
				for _, arg := range args {
					if strings.Contains(arg, "--cluster.peer") {
						count++
					}
				}
				assert.Equal(t, 3, count)
			},
		},
		"resources": {
			mco: func() *mcov1beta2.MultiClusterObservability {
				ret := makeBaseMco()
				ret.Spec.AdvancedConfig = &mcov1beta2.AdvancedConfig{
					Alertmanager: &mcov1beta2.CommonSpec{
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("500Mi"),
							},
						},
					},
				}
				return ret
			},
			expect: func(t *testing.T, sts *appsv1.StatefulSet) {
				assert.Equal(t, resource.MustParse("1"), sts.Spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceCPU])
				assert.Equal(t, resource.MustParse("1Gi"), sts.Spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceMemory])
				assert.Equal(t, resource.MustParse("500m"), sts.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU])
				assert.Equal(t, resource.MustParse("500Mi"), sts.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory])
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			clientCa := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "extension-apiserver-authentication",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"client-ca-file": "test",
				},
			}
			kubeClient := fake.NewClientBuilder().WithObjects(clientCa).Build()

			alertResources := renderTemplates(t, kubeClient, tc.mco())

			sts := getResource[*appsv1.StatefulSet](alertResources, "")
			tc.expect(t, sts)
		})
	}
}

func makeBaseMco() *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				StorageClass:            "gp2",
				AlertmanagerStorageSize: "1Gi",
			},
		},
	}
}

func renderTemplates(t *testing.T, kubeClient client.Client, mco *mcov1beta2.MultiClusterObservability) []*unstructured.Unstructured {
	wd, err := os.Getwd()
	assert.NoError(t, err)
	templatesPath := filepath.Join(wd, "..", "..", "manifests")
	os.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)
	defer os.Unsetenv(templatesutil.TemplatesPathEnvVar)

	config.ReadImageManifestConfigMap(kubeClient, "v1")
	renderer := NewMCORenderer(mco, kubeClient, nil)

	//load and render alertmanager templates
	alertTemplates, err := templates.GetOrLoadAlertManagerTemplates(templatesutil.GetTemplateRenderer())
	assert.NoError(t, err)
	alertResources, err := renderer.renderAlertManagerTemplates(alertTemplates, "namespace", map[string]string{"test": "test"})
	assert.NoError(t, err)

	return alertResources
}

func getResource[T runtime.Object](objects []*unstructured.Unstructured, name string) T {
	var ret T
	found := false

	if reflect.TypeOf(ret).Kind() != reflect.Ptr { // Ensure it's a pointer
		panic(fmt.Sprintf("expected a pointer, got %T", ret))
	}
	tType := reflect.TypeOf(ret).Elem() // Get the type of the object

	for _, obj := range objects {
		// check if the name matches
		if name != "" && obj.GetName() != name {
			continue
		}

		// convert unstructured object to typed object
		typedObject := reflect.New(tType).Interface().(T)
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, typedObject)
		if err != nil {
			panic(fmt.Sprintf("failed to convert %q to %T: %v", obj.GetName(), *new(T), err))
		}

		// check if the kind matches
		if !strings.Contains(tType.String(), obj.GetKind()) {
			continue
		}

		// check if we already found an object of this type. If so, panic
		if found {
			panic(fmt.Sprintf("found multiple objects of type %T", *new(T)))
		}

		found = true
		ret = typedObject
	}

	if !found {
		panic(fmt.Sprintf("could not find object of type %T", *new(T)))
	}

	return ret
}
