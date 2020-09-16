// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	cert "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	observatoriumv1alpha1 "github.com/observatorium/operator/api/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	placementv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

func init() {
	os.Setenv("TEMPLATES_PATH", "../../../manifests/")
}

func TestLabelsForMultiClusterMonitoring(t *testing.T) {
	lab := labelsForMultiClusterMonitoring("test")

	value, _ := lab["observability.open-cluster-management.io/name"]
	if value != "test" {
		t.Errorf("value (%v) is not the expected (test)", value)
	}
}

func createObservatoriumAPIService(name, namespace string) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-observatorium-observatorium-api",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "api",
				"app.kubernetes.io/instance":  name + "-observatorium",
			},
		},
		Spec: corev1.ServiceSpec{},
	}
}

func createReadyPod(name, namespace, podName string) *corev1.Pod {
	ready := corev1.PodCondition{
		Type: Ready,
	}
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"observability.open-cluster-management.io/name": name,
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				ready,
			},
		},
	}
}

func createReadyStatefulSet(name, namespace, statefulSetName string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: namespace,
			Labels: map[string]string{
				"observability.open-cluster-management.io/name": name,
			},
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 1,
		},
	}
}

func createReadyDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-fake-ready-deployment",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":                   "api",
				"app.kubernetes.io/instance":                    name + "-fake-deployment",
				"observability.open-cluster-management.io/name": name,
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     1,
			AvailableReplicas: 1,
		},
	}
}

func createFailedDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-fake-failed-deployment",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":                   "api",
				"app.kubernetes.io/instance":                    name + "-fake-deployment",
				"observability.open-cluster-management.io/name": name,
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 0,
		},
	}
}

func createClusterVersion() *configv1.ClusterVersion {
	return &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: configv1.ClusterID("xxx-xxxxxx-xxxx"),
		},
	}
}

func TestMultiClusterMonitoringCRUpdate(t *testing.T) {
	var (
		name      = "monitoring"
		namespace = mcoconfig.GetDefaultNamespace()
	)
	logf.SetLogger(logf.ZapLogger(true))

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	//provide a non-existence path to bypass the rendering
	//cannot convert unstructured.Unstructured into v1.Service in fake client
	os.Setenv("TEMPLATES_PATH", path.Join(wd, "../../../tests/manifests"))

	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: mcov1beta1.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta1.StorageConfigObject{
				MetricObjectStorage: &mcov1beta1.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
			},
		},
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta1.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)
	routev1.AddToScheme(s)
	placementv1.AddToScheme(s)
	cert.AddToScheme(s)

	svc := createObservatoriumAPIService(name, namespace)
	grafanaCert := newTestCert(GetGrafanaCerts(), namespace)
	serverCert := newTestCert(GetServerCerts(), namespace)

	objs := []runtime.Object{mco, svc, grafanaCert, serverCert}
	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(objs...)

	ocpClient := fakeconfigclient.NewSimpleClientset([]runtime.Object{createClusterVersion()}...)
	// Create a ReconcileMemcached object with the scheme and fake client.
	r := &ReconcileMultiClusterObservability{client: cl, scheme: s, ocpClient: ocpClient}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	// Create empty client
	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	updatedMCO := &mcov1beta1.MultiClusterObservability{}
	err = cl.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	if updatedMCO.Status.Conditions[0].Type != Failed {
		t.Fatalf("Failed to get correct MCO installing status, expect failed")
	}
	err = cl.Create(context.TODO(), createSecret("test", "test", namespace))
	if err != nil {
		t.Fatalf("Failed to create secret: (%v)", err)
	}
	// Update client with 1 pod 1 statefulSet
	grafanaPod := createReadyPod(name, namespace, "grafana")
	err = cl.Create(context.TODO(), grafanaPod)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	observatoriumThanosCompactSS := createReadyStatefulSet(name, namespace, name+"-observatorium-thanos-compact")
	err = cl.Create(context.TODO(), observatoriumThanosCompactSS)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}

	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	updatedMCO = &mcov1beta1.MultiClusterObservability{}
	err = r.client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	// Update client with all pods all statefulSet
	observatoriumobservatoriumapiPod := createReadyPod(name, namespace, name+"-observatorium-observatorium-api")
	err = cl.Create(context.TODO(), observatoriumobservatoriumapiPod)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	observatoriumthanosqueryPod := createReadyPod(name, namespace, name+"-observatorium-thanos-query")
	err = cl.Create(context.TODO(), observatoriumthanosqueryPod)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	observatoriumthanosreceivecontrollerPod := createReadyPod(name, namespace, name+"-observatorium-thanos-receive-controller")
	err = cl.Create(context.TODO(), observatoriumthanosreceivecontrollerPod)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	observatoriumthanosreceivedefaultSS := createReadyStatefulSet(name, namespace, name+"-observatorium-thanos-receive-default")
	err = cl.Create(context.TODO(), observatoriumthanosreceivedefaultSS)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	observatoriumthanosruleSS := createReadyStatefulSet(name, namespace, name+"-observatorium-thanos-rule")
	err = cl.Create(context.TODO(), observatoriumthanosruleSS)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	observatoriumthanosstorememcachedSS := createReadyStatefulSet(name, namespace, name+"-observatorium-thanos-store-memcached")
	err = cl.Create(context.TODO(), observatoriumthanosstorememcachedSS)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	observatoriumthanosstoreshard0SS := createReadyStatefulSet(name, namespace, name+"-observatorium-thanos-store-shard-0")
	err = cl.Create(context.TODO(), observatoriumthanosstoreshard0SS)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}

	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	updatedMCO = &mcov1beta1.MultiClusterObservability{}
	err = r.client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	if updatedMCO.Status.Conditions[0].Message != "No deployment found." {
		t.Fatalf("Failed to get correct MCO installing status, expect Installed and showing Failed message")
	}

	readyDeployment := createReadyDeployment(name, namespace)
	err = cl.Create(context.TODO(), readyDeployment)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	log.Info("updated MultiClusterObservability successfully", "MultiClusterObservability", updatedMCO)
	updatedMCO = &mcov1beta1.MultiClusterObservability{}
	err = r.client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	if updatedMCO.Status.Conditions[0].Type != Ready {
		t.Fatalf("Failed to get correct MCO status, expect Ready")
	}
	log.Info("updated MultiClusterObservability successfully", "MultiClusterObservability", updatedMCO)

	failedDeployment := createFailedDeployment(name, namespace)
	err = cl.Create(context.TODO(), failedDeployment)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	log.Info("updated MultiClusterObservability successfully", "MultiClusterObservability", updatedMCO)
	updatedMCO = &mcov1beta1.MultiClusterObservability{}
	err = r.client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	if updatedMCO.Status.Conditions[0].Message != "Deployment is failed for monitoring-fake-failed-deployment" {
		t.Fatalf("Failed to get correct MCO status, expect failed with failed deployment")
	}
	log.Info("updated MultiClusterObservability successfully", "MultiClusterObservability", updatedMCO)

	//Test finalizer
	mco.ObjectMeta.DeletionTimestamp = &v1.Time{time.Now()}
	mco.ObjectMeta.Finalizers = []string{certFinalizer, "test-finalizerr"}
	err = cl.Update(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	_, err = r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile for finalizer: (%v)", err)
	}
}

func createSecret(key, name, namespace string) *corev1.Secret {

	s3Conf := &mcoconfig.ObjectStorgeConf{
		Type: "s3",
		Config: mcoconfig.Config{
			Bucket:    "bucket",
			Endpoint:  "endpoint",
			Insecure:  true,
			AccessKey: "access_key",
			SecretKey: "secret_key`",
		},
	}
	configYaml, _ := yaml.Marshal(s3Conf)

	configYamlMap := map[string][]byte{}
	configYamlMap[key] = configYaml

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: "Opaque",
		Data: configYamlMap,
	}
}

func TestCheckS3Conf(t *testing.T) {
	mco := &mcov1beta1.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
		Spec: mcov1beta1.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta1.StorageConfigObject{
				MetricObjectStorage: &mcov1beta1.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
			},
		},
	}

	s := scheme.Scheme
	mcov1beta1.SchemeBuilder.AddToScheme(s)
	objs := []runtime.Object{mco}
	c := fake.NewFakeClient(objs...)
	mcoCondition := CheckS3Conf(c, mco)
	if mcoCondition == nil {
		t.Errorf("check s3 conf failed: got %v, expected non-nil", mcoCondition)
	}

	err := c.Create(context.TODO(), createSecret("test", "test", "test"))
	if err != nil {
		t.Fatalf("Failed to create secret: (%v)", err)
	}

	mcoCondition = CheckS3Conf(c, mco)
	if mcoCondition != nil {
		t.Errorf("check s3 conf failed: got %v, expected nil", mcoCondition)
	}

	err = c.Update(context.TODO(), createSecret("error", "test", "test"))
	if err != nil {
		t.Fatalf("Failed to update secret: (%v)", err)
	}

	mcoCondition = CheckS3Conf(c, mco)
	if mcoCondition == nil {
		t.Errorf("check s3 conf failed: got %v, expected no-nil", mcoCondition)
	}
}
