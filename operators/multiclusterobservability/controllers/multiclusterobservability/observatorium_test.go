// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"bytes"
	"context"
	"testing"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoutil "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	observatoriumv1alpha1 "github.com/stolostron/observatorium-operator/api/v1alpha1"
)

var (
	storageClassName = ""
)

func TestNewVolumeClaimTemplate(t *testing.T) {
	vct := newVolumeClaimTemplate("10Gi", "test")
	if vct.Spec.AccessModes[0] != corev1.ReadWriteOnce ||
		vct.Spec.Resources.Requests[corev1.ResourceStorage] != resource.MustParse("10Gi") {
		t.Errorf("Failed to newVolumeClaimTemplate")
	}
}

func TestNewDefaultObservatoriumSpec(t *testing.T) {
	statefulSetSize := "1Gi"
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Annotations: map[string]string{
				mcoconfig.AnnotationKeyImageRepository: "quay.io:443/acm-d",
				mcoconfig.AnnotationKeyImageTagSuffix:  "tag",
			},
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:           "key",
					Name:          "name",
					TLSSecretName: "secret",
				},
				WriteStorage: []*mcoshared.PreConfiguredStorage{
					{
						Key:  "write_key",
						Name: "write_name",
					},
				},
				StorageClass:            storageClassName,
				AlertmanagerStorageSize: "1Gi",
				CompactStorageSize:      "1Gi",
				RuleStorageSize:         "1Gi",
				ReceiveStorageSize:      "1Gi",
				StoreStorageSize:        "1Gi",
			},
			ObservabilityAddonSpec: &mcoshared.ObservabilityAddonSpec{
				EnableMetrics: true,
				Interval:      300,
			},
		},
	}

	writeStorageS := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "write_name",
			Namespace: mcoconfig.GetDefaultNamespace(),
		},
		Type: "Opaque",
		Data: map[string][]byte{
			"write_key": []byte(`url: http://remotewrite/endpoint
`),
		},
	}

	objs := []runtime.Object{mco, writeStorageS}
	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

	obs, _ := newDefaultObservatoriumSpec(cl, mco, storageClassName, "")

	receiversStorage := obs.Thanos.Receivers.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	ruleStorage := obs.Thanos.Rule.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	storeStorage := obs.Thanos.Store.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	compactStorage := obs.Thanos.Compact.VolumeClaimTemplate.Spec.Resources.Requests["storage"]
	obs, _ = newDefaultObservatoriumSpec(cl, mco, storageClassName, "")
	if *obs.Thanos.Receivers.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Thanos.Rule.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Thanos.Store.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		*obs.Thanos.Compact.VolumeClaimTemplate.Spec.StorageClassName != storageClassName ||
		receiversStorage.String() != statefulSetSize ||
		ruleStorage.String() != statefulSetSize ||
		storeStorage.String() != statefulSetSize ||
		compactStorage.String() != statefulSetSize ||
		obs.ObjectStorageConfig.Thanos.Key != "key" ||
		obs.ObjectStorageConfig.Thanos.Name != "name" ||
		obs.ObjectStorageConfig.Thanos.TLSSecretName != "secret" ||
		obs.Thanos.Query.LookbackDelta != "600s" ||
		obs.API.AdditionalWriteEndpoints.EndpointsConfigSecret != endpointsConfigName {
		t.Errorf("Failed to newDefaultObservatorium")
	}

	endpointS := &corev1.Secret{}
	err := cl.Get(context.TODO(), types.NamespacedName{
		Name:      endpointsConfigName,
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, endpointS)
	if err != nil {
		t.Errorf("Failed to get endpoint config secret due to %v", err)
	}
	endpointConfig := []mcoutil.RemoteWriteEndpointWithSecret{}
	err = yaml.Unmarshal(endpointS.Data[endpointsKey], &endpointConfig)
	if err != nil {
		t.Errorf("Failed to unmarshal endpoint secret due to %v", err)
	}
	if endpointConfig[0].Name != "write_name" || endpointConfig[0].URL.String() != "http://remotewrite/endpoint" {
		t.Errorf("Wrong endpoint config: %s, %s", endpointConfig[0].Name, endpointConfig[0].URL.String())
	}
}

func TestNoUpdateObservatoriumCR(t *testing.T) {
	var (
		namespace = mcoconfig.GetDefaultNamespace()
	)

	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoconfig.GetDefaultCRName(),
			Annotations: map[string]string{
				mcoconfig.AnnotationKeyImageTagSuffix: "tag",
			},
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				StorageClass:            storageClassName,
				AlertmanagerStorageSize: "1Gi",
				CompactStorageSize:      "1Gi",
				RuleStorageSize:         "1Gi",
				ReceiveStorageSize:      "1Gi",
				StoreStorageSize:        "1Gi",
			},
			ObservabilityAddonSpec: &mcoshared.ObservabilityAddonSpec{
				EnableMetrics: true,
				Interval:      300,
			},
		},
	}
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)

	objs := []runtime.Object{mco}
	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	mcoconfig.SetOperandNames(cl)

	_, err := GenerateObservatoriumCR(cl, s, mco)
	if err != nil {
		t.Errorf("Failed to create observatorium due to %v", err)
	}

	// Check if this Observatorium CR already exists
	observatoriumCRFound := &observatoriumv1alpha1.Observatorium{}
	cl.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      mcoconfig.GetDefaultCRName(),
			Namespace: namespace,
		},
		observatoriumCRFound,
	)

	oldSpec := observatoriumCRFound.Spec
	newSpec, _ := newDefaultObservatoriumSpec(cl, mco, storageClassName, "")
	oldSpecBytes, _ := yaml.Marshal(oldSpec)
	newSpecBytes, _ := yaml.Marshal(newSpec)

	if res := bytes.Compare(newSpecBytes, oldSpecBytes); res != 0 {
		t.Errorf("%v should be equal to %v", string(oldSpecBytes), string(newSpecBytes))
	}

	_, err = GenerateObservatoriumCR(cl, s, mco)
	if err != nil {
		t.Errorf("Failed to update observatorium due to %v", err)
	}
}

func TestGetTLSSecretMountPath(t *testing.T) {

	testCaseList := []struct {
		name        string
		secret      *corev1.Secret
		storeConfig *mcoshared.PreConfiguredStorage
		expected    string
	}{

		{
			"no tls secret defined",
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: mcoconfig.GetDefaultNamespace(),
				},
				Type: "Opaque",
				Data: map[string][]byte{
					"thanos.yaml": []byte(`type: s3
config:
  bucket: s3
  endpoint: s3.amazonaws.com
`),
				},
			},
			&mcoshared.PreConfiguredStorage{
				Key:  "thanos.yaml",
				Name: "test",
			},
			"",
		},
		{
			"has tls config defined",
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-1",
					Namespace: mcoconfig.GetDefaultNamespace(),
				},
				Type: "Opaque",
				Data: map[string][]byte{
					"thanos.yaml": []byte(`type: s3
config:
  bucket: s3
  endpoint: s3.amazonaws.com
  insecure: true
  http_config:
    tls_config:
      ca_file: /etc/minio/certs/ca.crt
      cert_file: /etc/minio/certs/public.crt
      key_file: /etc/minio/certs/private.key
      insecure_skip_verify: true
`),
				},
			},
			&mcoshared.PreConfiguredStorage{
				Key:  "thanos.yaml",
				Name: "test-1",
			},
			"/etc/minio/certs",
		},
		{
			"has tls config defined in root path",
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-2",
					Namespace: mcoconfig.GetDefaultNamespace(),
				},
				Type: "Opaque",
				Data: map[string][]byte{
					"thanos.yaml": []byte(`type: s3
config:
  bucket: s3
  endpoint: s3.amazonaws.com
  insecure: true
  http_config:
    tls_config:
      ca_file: /ca.crt
      cert_file: /etc/minio/certs/public.crt
      key_file: /etc/minio/certs/private.key
      insecure_skip_verify: true
`),
				},
			},
			&mcoshared.PreConfiguredStorage{
				Key:  "thanos.yaml",
				Name: "test-2",
			},
			"/",
		},
	}

	client := fake.NewClientBuilder().Build()
	for _, c := range testCaseList {
		err := client.Create(context.TODO(), c.secret)
		if err != nil {
			t.Errorf("failed to create object storage secret, due to %v", err)
		}
		path, err := getTLSSecretMountPath(client, c.storeConfig)
		if err != nil {
			t.Errorf("failed to get tls secret mount path, due to %v", err)
		}
		if path != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, path, c.expected)
		}
	}
}
