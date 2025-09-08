// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	routev1 "github.com/openshift/api/route/v1"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoutil "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	observatoriumv1alpha1 "github.com/stolostron/observatorium-operator/api/v1alpha1"

	"gopkg.in/yaml.v2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			"write_key": []byte(`url: http://remotewrite/endpoint`),
		},
	}

	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.SchemeBuilder.AddToScheme(s)

	objs := []runtime.Object{mco, writeStorageS}
	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).Build()

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

func TestNewDefaultObservatoriumSpecWithTShirtSize(t *testing.T) {
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
			InstanceSize: mcoconfig.FourXLarge,
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
			"write_key": []byte(`url: http://remotewrite/endpoint`),
		},
	}
	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.SchemeBuilder.AddToScheme(s)

	objs := []runtime.Object{mco, writeStorageS}
	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).Build()

	obs, err := newDefaultObservatoriumSpec(cl, mco, storageClassName, "")
	if err != nil {
		t.Errorf("failed to create obs spec")
	}

	if obs.Thanos.Receivers.Resources.Requests.Cpu().String() != "10" ||
		obs.Thanos.Receivers.Resources.Requests.Memory().String() != "128Gi" ||
		*obs.Thanos.Receivers.Replicas != 12 ||
		obs.Thanos.Query.Resources.Requests.Cpu().String() != "7" ||
		obs.Thanos.Query.Resources.Requests.Memory().String() != "18Gi" ||
		*obs.Thanos.Query.Replicas != 10 ||
		obs.Thanos.QueryFrontend.Resources.Requests.Cpu().String() != "4" ||
		obs.Thanos.QueryFrontend.Resources.Requests.Memory().String() != "12Gi" ||
		*obs.Thanos.QueryFrontend.Replicas != 10 ||
		obs.Thanos.Compact.Resources.Requests.Cpu().String() != "6" ||
		obs.Thanos.Compact.Resources.Requests.Memory().String() != "18Gi" ||
		*obs.Thanos.Compact.Replicas != 1 ||
		obs.Thanos.Rule.Resources.Requests.Cpu().String() != "6" ||
		obs.Thanos.Rule.Resources.Requests.Memory().String() != "15Gi" ||
		*obs.Thanos.Rule.Replicas != 3 ||
		obs.Thanos.Store.Resources.Requests.Cpu().String() != "6" ||
		obs.Thanos.Store.Resources.Requests.Memory().String() != "20Gi" ||
		*obs.Thanos.Store.Shards != 6 {
		t.Errorf("Failed t-shirt size for Obs Spec")
	}
}

func TestUpdateObservatoriumCR(t *testing.T) {
	namespace := mcoconfig.GetDefaultNamespace()

	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoconfig.GetDefaultCRName(),
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
	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)

	objs := []runtime.Object{mco}
	objs = append(objs, []runtime.Object{
		&corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Name: mcoconfig.GetOperandNamePrefix() + mcoconfig.ObservatoriumAPI, Namespace: namespace},
			Data: map[string]string{
				"config.yaml": "test",
			},
		},
	}...)

	// Create a fake client to mock API calls.
	// This should have no extra objects beyond the CMO CRD.
	noConfigCl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(mco).Build()
	mcoconfig.SetOperandNames(noConfigCl)

	_, err := GenerateObservatoriumCR(noConfigCl, s, mco)
	if err != nil {
		t.Errorf("Failed to create observatorium due to %v", err)
	}

	// Check if this Observatorium CR already exists
	createdObservatoriumCR := &observatoriumv1alpha1.Observatorium{}
	noConfigCl.Get(context.TODO(), types.NamespacedName{
		Name:      mcoconfig.GetDefaultCRName(),
		Namespace: namespace,
	}, createdObservatoriumCR)
	hash, configHashFound := createdObservatoriumCR.Labels[obsCRConfigHashLabelName]
	if !configHashFound {
		t.Errorf("config-hash label not found in Observatorium CR")
	}
	const observatoriumEmptyConfigHash = "8a80554c91d9fca8acb82f023de02f11"
	if hash != observatoriumEmptyConfigHash {
		t.Errorf("config-hash label contains unexpected hash. Want: '%s', got '%s'", observatoriumEmptyConfigHash, hash)
	}

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(append(objs, createdObservatoriumCR)...).Build()
	mcoconfig.SetOperandNames(cl)

	_, err = GenerateObservatoriumCR(cl, s, mco)
	if err != nil {
		t.Errorf("Failed to update observatorium due to %v", err)
	}
	objs = append(objs, []runtime.Object{createdObservatoriumCR}...)
	updatedObservatorium := &observatoriumv1alpha1.Observatorium{}
	cl.Get(context.TODO(), types.NamespacedName{
		Name:      mcoconfig.GetDefaultCRName(),
		Namespace: namespace,
	}, updatedObservatorium)
	updatedHash, updatedHashFound := updatedObservatorium.Labels[obsCRConfigHashLabelName]
	if !updatedHashFound {
		t.Errorf("config-hash label not found in Observatorium CR")
	}

	const expectedConfigHash = "321c72e32663033537aa77c56d90834b"
	if updatedHash != expectedConfigHash {
		t.Errorf("config-hash label contains unexpected hash. Want: '%s', got '%s'", expectedConfigHash, updatedHash)
	}

	createdSpecBytes, _ := yaml.Marshal(createdObservatoriumCR.Spec)
	updatedSpecBytes, _ := yaml.Marshal(updatedObservatorium.Spec)
	if res := bytes.Compare(updatedSpecBytes, createdSpecBytes); res != 0 {
		t.Errorf("%v should be equal to %v", string(createdSpecBytes), string(updatedSpecBytes))
	}

}

func TestTShirtSizeUpdateObservatoriumCR(t *testing.T) {
	namespace := mcoconfig.GetDefaultNamespace()

	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoconfig.GetDefaultCRName(),
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			InstanceSize: mcoconfig.Large,
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
	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)

	// Create a fake client to mock API calls.
	// This should have no extra objects beyond the CMO CRD.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(mco).Build()
	mcoconfig.SetOperandNames(cl)

	_, err := GenerateObservatoriumCR(cl, s, mco)
	if err != nil {
		t.Errorf("Failed to create observatorium due to %v", err)
	}

	// Check if this Observatorium CR already exists
	createdObservatoriumCR := &observatoriumv1alpha1.Observatorium{}
	cl.Get(context.TODO(), types.NamespacedName{
		Name:      mcoconfig.GetDefaultCRName(),
		Namespace: namespace,
	}, createdObservatoriumCR)

	if createdObservatoriumCR.Spec.Thanos.Receivers.Resources.Requests.Cpu().String() != "5" ||
		createdObservatoriumCR.Spec.Thanos.Receivers.Resources.Requests.Memory().String() != "24Gi" ||
		*createdObservatoriumCR.Spec.Thanos.Receivers.Replicas != 6 {
		t.Errorf("t-shirt size values for receive not correct")
	}

	mco.Spec.InstanceSize = mcoconfig.TwoXLarge
	_, err = GenerateObservatoriumCR(cl, s, mco)
	if err != nil {
		t.Errorf("Failed to update observatorium due to %v", err)
	}

	updatedObservatorium := &observatoriumv1alpha1.Observatorium{}
	cl.Get(context.TODO(), types.NamespacedName{
		Name:      mcoconfig.GetDefaultCRName(),
		Namespace: namespace,
	}, updatedObservatorium)

	if updatedObservatorium.Spec.Thanos.Receivers.Resources.Requests.Cpu().String() != "6" ||
		updatedObservatorium.Spec.Thanos.Receivers.Resources.Requests.Memory().String() != "52Gi" ||
		*updatedObservatorium.Spec.Thanos.Receivers.Replicas != 12 {
		t.Errorf("updated t-shirt size values for receive not correct")
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
	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)

	objs := []runtime.Object{mco}
	objs = append(objs, []runtime.Object{
		&corev1.Secret{
			TypeMeta:   metav1.TypeMeta{Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{Name: mcoconfig.ServerCerts, Namespace: namespace},
			Data: map[string][]byte{
				"tls.crt": []byte("server-cert"),
				"tls.key": []byte("server-key"),
			},
		},
		&corev1.Secret{
			TypeMeta:   metav1.TypeMeta{Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{Name: mcoconfig.ClientCACerts, Namespace: namespace},
			Data: map[string][]byte{
				"tls.crt": []byte("client-ca-cert"),
			},
		},
		&corev1.Secret{
			TypeMeta:   metav1.TypeMeta{Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{Name: mcoconfig.GetOperandNamePrefix() + mcoconfig.ObservatoriumAPI, Namespace: namespace},
			Data: map[string][]byte{
				"tls.crt": []byte("test"),
			},
		},
		&corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Name: mcoconfig.GetOperandNamePrefix() + mcoconfig.ObservatoriumAPI, Namespace: namespace},
			Data: map[string]string{
				"config.yaml": "test",
			},
		},
	}...)

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).Build()
	mcoconfig.SetOperandNames(cl)

	_, err := GenerateObservatoriumCR(cl, s, mco)
	if err != nil {
		t.Errorf("Failed to create observatorium due to %v", err)
	}

	// Check if this Observatorium CR already exists
	createdObservatoriumCR := &observatoriumv1alpha1.Observatorium{}
	cl.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      mcoconfig.GetDefaultCRName(),
			Namespace: namespace,
		},
		createdObservatoriumCR,
	)

	hash, configHashFound := createdObservatoriumCR.Labels["config-hash"]
	if !configHashFound {
		t.Errorf("config-hash label not found in Observatorium CR")
	}
	const expectedConfigHash = "321c72e32663033537aa77c56d90834b"
	if hash != expectedConfigHash {
		t.Errorf("config-hash label contains unexpected hash. Want: '%s', got '%s'", expectedConfigHash, hash)
	}

	oldSpecBytes, _ := yaml.Marshal(createdObservatoriumCR.Spec)
	newSpec, _ := newDefaultObservatoriumSpec(cl, mco, storageClassName, "")
	newSpecBytes, _ := yaml.Marshal(newSpec)

	if res := bytes.Compare(newSpecBytes, oldSpecBytes); res != 0 {
		t.Errorf("%v should be equal to %v", string(oldSpecBytes), string(newSpecBytes))
	}

	_, err = GenerateObservatoriumCR(cl, s, mco)
	if err != nil {
		t.Errorf("Failed to update observatorium due to %v", err)
	}
}

func TestHashObservatoriumCRWithConfig(t *testing.T) {
	namespace := mcoconfig.GetDefaultNamespace()

	tt := []struct {
		name         string
		objs         []runtime.Object
		expectedHash string
		expectedErr  error
	}{
		{
			name:         "With Observatorium's secrets and configmap present",
			expectedHash: "321c72e32663033537aa77c56d90834b",
			objs: []runtime.Object{
				&corev1.ConfigMap{
					TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap"},
					ObjectMeta: metav1.ObjectMeta{Name: mcoconfig.GetOperandNamePrefix() + mcoconfig.ObservatoriumAPI, Namespace: namespace},
					Data: map[string]string{
						"config.yaml": "test",
					},
				},
			},
		},
		{
			name: "Without Observatorium's secrets and configmap present",
			// The hash is still calculated when the configmap isn't present, because the implementation
			// is hashing an empty object if it isn't found.
			expectedHash: "8a80554c91d9fca8acb82f023de02f11",
			objs:         []runtime.Object{},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// Create a fake client to mock API calls.
			cl := fake.NewClientBuilder().WithRuntimeObjects(tc.objs...).Build()
			hash, err := hashObservatoriumCRConfig(cl)

			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("unexpected error: %v\nwant: %v", err, tc.expectedErr)
			}

			if hash != tc.expectedHash {
				t.Errorf("config-hash label contains unexpected hash. Want: '%s', got '%s'", tc.expectedHash, hash)
			}
		})
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

func TestObservatoriumCustomArgs(t *testing.T) {
	receiveTestArgs := []string{"receive", "--arg1", "--args2"}
	storeTestArgs := []string{"store", "--arg1", "--args2"}
	queryTestArgs := []string{"query", "--arg1", "--args2"}
	ruleTestArgs := []string{"rule", "--arg1", "--args2"}
	compactTestArgs := []string{"compact", "--arg1", "--args2"}
	queryFrontendTestArgs := []string{"queryfrontend", "--arg1", "--args2"}
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
			AdvancedConfig: &mcov1beta2.AdvancedConfig{
				Receive: &mcov1beta2.ReceiveSpec{
					Containers: []corev1.Container{
						{
							Args: receiveTestArgs,
						},
					},
				},
				Store: &mcov1beta2.StoreSpec{
					Containers: []corev1.Container{
						{
							Args: storeTestArgs,
						},
					},
				},
				Query: &mcov1beta2.QuerySpec{
					Containers: []corev1.Container{
						{
							Args: queryTestArgs,
						},
					},
				},
				Rule: &mcov1beta2.RuleSpec{
					Containers: []corev1.Container{
						{
							Args: ruleTestArgs,
						},
					},
				},
				Compact: &mcov1beta2.CompactSpec{
					Containers: []corev1.Container{
						{
							Args: compactTestArgs,
						},
					},
				},
				QueryFrontend: &mcov1beta2.QueryFrontendSpec{
					Containers: []corev1.Container{
						{
							Args: queryFrontendTestArgs,
						},
					},
				},
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
	if !reflect.DeepEqual(obs.Thanos.Receivers.Containers[0].Args, receiveTestArgs) {
		t.Errorf("Failed to propagate custom args to Receive Observatorium spec")
	}
	if !reflect.DeepEqual(obs.Thanos.Store.Containers[0].Args, storeTestArgs) {
		t.Errorf("Failed to propagate custom args to Store Observatorium spec")
	}
	if !reflect.DeepEqual(obs.Thanos.Query.Containers[0].Args, queryTestArgs) {
		t.Errorf("Failed to propagate custom args to Query Observatorium spec")
	}
	if !reflect.DeepEqual(obs.Thanos.Rule.Containers[0].Args, ruleTestArgs) {
		t.Errorf("Failed to propagate custom args to Rule Observatorium spec")
	}
	if !reflect.DeepEqual(obs.Thanos.Compact.Containers[0].Args, compactTestArgs) {
		t.Errorf("Failed to propagate custom args to Compact Observatorium spec")
	}
	if !reflect.DeepEqual(obs.Thanos.QueryFrontend.Containers[0].Args, queryFrontendTestArgs) {
		t.Errorf("Failed to propagate custom args to QueryFrontend Observatorium spec")
	}
}

func TestGenerateAPIGatewayRoute(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	s.AddKnownTypes(mcov1beta2.GroupVersion)
	if err := mcov1beta2.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add scheme: (%v)", err)
	}

	clientScheme := runtime.NewScheme()
	if err := routev1.AddToScheme(clientScheme); err != nil {
		t.Fatalf("Unable to add route scheme: (%v)", err)
	}

	want := routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAPIGateway,
			Namespace: mcoconfig.GetDefaultNamespace(),
		},
		Spec: routev1.RouteSpec{
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString(obsApiGatewayTargetPort),
			},
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: mcoconfig.GetOperandNamePrefix() + obsAPIGateway,
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationPassthrough,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyNone,
			},
		},
	}

	tests := []struct {
		name     string
		want     routev1.Route
		c        client.WithWatch
		instance *mcov1beta2.MultiClusterObservability
	}{
		{
			name:     "Test create a Route if it does not exist",
			want:     want,
			c:        fake.NewClientBuilder().WithScheme(clientScheme).Build(),
			instance: &mcov1beta2.MultiClusterObservability{},
		},
		{
			name:     "Test update a Route if it has been modified",
			want:     want,
			instance: &mcov1beta2.MultiClusterObservability{},
			c: fake.NewClientBuilder().WithScheme(clientScheme).WithObjects(&routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obsAPIGateway,
					Namespace: mcoconfig.GetDefaultNamespace(),
				},
				Spec: routev1.RouteSpec{
					Port: &routev1.RoutePort{
						TargetPort: intstr.FromString("oauth-proxy"),
					},
					To: routev1.RouteTargetReference{
						Kind: "Service",
						Name: "modified",
					},
					TLS: &routev1.TLSConfig{
						Termination:                   routev1.TLSTerminationReencrypt,
						InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
					},
				},
			}).Build(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GenerateAPIGatewayRoute(ctx, tt.c, s, tt.instance)
			if err != nil {
				t.Errorf("GenerateAPIGatewayRoute() error = %v", err)
				return
			}
			list := &routev1.RouteList{}
			if err := tt.c.List(context.Background(), list); err != nil {
				t.Fatalf("Unable to list routes: (%v)", err)
			}
			if len(list.Items) != 1 {
				t.Fatalf("Expected 1 route, got %d", len(list.Items))
			}
			if !reflect.DeepEqual(list.Items[0].Spec, tt.want.Spec) {
				t.Fatalf("Expected route spec: %v, got %v", tt.want.Spec, list.Items[0].Spec)
			}
		})
	}
}
