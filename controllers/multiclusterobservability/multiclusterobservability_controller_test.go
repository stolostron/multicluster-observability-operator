// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	cert "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	observatoriumv1alpha1 "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	placementv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
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
			Name:      name + "-observatorium-api",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "api",
				"app.kubernetes.io/instance":  name,
			},
		},
		Spec: corev1.ServiceSpec{},
	}
}

func newClusterManagementAddon() *addonv1alpha1.ClusterManagementAddOn {
	return &addonv1alpha1.ClusterManagementAddOn{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ClusterManagementAddOn",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "ObservabilityController",
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			AddOnMeta: addonv1alpha1.AddOnMeta{
				DisplayName: "ObservabilityController",
				Description: "ObservabilityController Description",
			},
			AddOnConfiguration: addonv1alpha1.ConfigCoordinates{
				CRDName: "observabilityaddons.observability.open-cluster-management.io",
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

func createFailedStatefulSet(name, namespace, statefulSetName string) *appsv1.StatefulSet {
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
			ReadyReplicas: 0,
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
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":                   "api",
				"app.kubernetes.io/instance":                    name,
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
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":                   "api",
				"app.kubernetes.io/instance":                    name,
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

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	//provide a non-existence path to bypass the rendering
	//cannot convert unstructured.Unstructured into v1.Service in fake client
	os.Setenv("TEMPLATES_PATH", path.Join(wd, "../../../tests/manifests"))

	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				mcoconfig.AnnotationKeyImageTagSuffix: "tag",
			},
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfigObject{
				MetricObjectStorage: &mcov1beta2.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				StorageClass:            "gp2",
				AlertmanagerStorageSize: "1Gi",
				CompactStorageSize:      "1Gi",
				RuleStorageSize:         "1Gi",
				ReceiveStorageSize:      "1Gi",
				StoreStorageSize:        "1Gi",
			},
			RetentionConfig: &mcov1beta2.RetentionConfig{
				RetentionResolutionRaw: "1h",
				RetentionResolution5m:  "1h",
				RetentionResolution1h:  "1h",
			},
			ObservabilityAddonSpec: &mcov1beta2.ObservabilityAddonSpec{
				EnableMetrics: false,
			},
		},
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)
	routev1.AddToScheme(s)
	placementv1.AddToScheme(s)
	cert.AddToScheme(s)
	addonv1alpha1.AddToScheme(s)

	svc := createObservatoriumAPIService(name, namespace)
	grafanaCert := newTestCert(GetGrafanaCerts(), namespace)
	serverCert := newTestCert(GetServerCerts(), namespace)
	clustermgmtAddon := newClusterManagementAddon()

	objs := []runtime.Object{mco, svc, grafanaCert, serverCert, clustermgmtAddon}
	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(objs...)

	ocpClient := fakeconfigclient.NewSimpleClientset([]runtime.Object{createClusterVersion()}...)
	// Create a ReconcileMemcached object with the scheme and fake client.
	r := &MultiClusterObservabilityReconciler{Client: cl, Scheme: s, OcpClient: ocpClient}
	config.SetMonitoringCRName(name)
	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: name,
		},
	}

	// Create empty client
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	updatedMCO := &mcov1beta2.MultiClusterObservability{}
	err = cl.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	status := findStatusCondition(updatedMCO.Status.Conditions, "Failed")
	if status == nil || status.Reason != "ObjectStorageSecretNotFound" {
		t.Errorf("Failed to get correct MCO status, expect Failed")
	}

	err = cl.Create(context.TODO(), createSecret("test", "test", namespace))
	if err != nil {
		t.Fatalf("Failed to create secret: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	status = findStatusCondition(updatedMCO.Status.Conditions, "Failed")
	if status == nil || status.Reason != "DeploymentNotFound" {
		t.Errorf("Failed to get correct MCO status, expect Failed")
	}
	expectedDeploymentNames := getExpectedDeploymentNames(name)
	for _, deployName := range expectedDeploymentNames {
		deploy := createReadyDeployment(deployName, namespace)
		err = cl.Create(context.TODO(), deploy)
		if err != nil {
			t.Fatalf("Failed to create deployment %s: %v", deployName, err)
		}
	}

	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	status = findStatusCondition(updatedMCO.Status.Conditions, "Failed")
	if status == nil || status.Reason != "StatefulSetNotFound" {
		t.Errorf("Failed to get correct MCO status, expect Failed")
	}

	expectedStatefulSetNames := getExpectedStatefulSetNames(name)
	for _, statefulName := range expectedStatefulSetNames {
		deploy := createReadyStatefulSet(name, namespace, statefulName)
		err = cl.Create(context.TODO(), deploy)
		if err != nil {
			t.Fatalf("Failed to create stateful set %s: %v", statefulName, err)
		}
	}

	result, err := r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	if result.Requeue {
		_, err = r.Reconcile(context.TODO(), req)
		if err != nil {
			t.Fatalf("reconcile: (%v)", err)
		}
	}

	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	status = findStatusCondition(updatedMCO.Status.Conditions, "Ready")
	if status == nil || status.Reason != "Ready" {
		t.Errorf("Failed to get correct MCO status, expect Ready")
	}

	status = findStatusCondition(updatedMCO.Status.Conditions, "MetricsDisabled")
	if status == nil || status.Reason != "MetricsDisabled" {
		t.Errorf("Failed to get correct MCO status, expect MetricsDisabled")
	}

	// test MetricsDisabled status
	err = cl.Delete(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to delete mco: (%v)", err)
	}
	mco.Spec.ObservabilityAddonSpec.EnableMetrics = true
	mco.ObjectMeta.ResourceVersion = ""
	err = cl.Create(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to create mco: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	status = findStatusCondition(updatedMCO.Status.Conditions, "MetricsDisabled")
	if status != nil {
		t.Errorf("Should have not MetricsDisabled status")
	}

	// test StatefulSetNotReady status
	err = cl.Delete(context.TODO(), createReadyStatefulSet(name, namespace, "alertmanager"))
	if err != nil {
		t.Fatalf("Failed to delete alertmanager: (%v)", err)
	}
	failedAlertManager := createFailedStatefulSet(name, namespace, "alertmanager")
	err = cl.Create(context.TODO(), failedAlertManager)
	if err != nil {
		t.Fatalf("Failed to create alertmanager: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	status = findStatusCondition(updatedMCO.Status.Conditions, "Ready")
	if status == nil || status.Reason != "Ready" {
		t.Errorf("Failed to get correct MCO status, expect Ready")
	}

	// test DeploymentNotReady status
	err = cl.Delete(context.TODO(), createReadyDeployment("rbac-query-proxy", namespace))
	if err != nil {
		t.Fatalf("Failed to delete rbac-query-proxy: (%v)", err)
	}
	err = cl.Delete(context.TODO(), failedAlertManager)
	if err != nil {
		t.Fatalf("Failed to delete alertmanager: (%v)", err)
	}
	err = cl.Create(context.TODO(), createReadyStatefulSet(name, namespace, "alertmanager"))
	if err != nil {
		t.Fatalf("Failed to delete alertmanager: (%v)", err)
	}

	failedRbacProxy := createFailedDeployment("rbac-query-proxy", namespace)
	err = cl.Create(context.TODO(), failedRbacProxy)
	if err != nil {
		t.Fatalf("Failed to create rbac-query-proxy: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	status = findStatusCondition(updatedMCO.Status.Conditions, "Ready")
	if status == nil || status.Reason != "Ready" {
		t.Errorf("Failed to get correct MCO status, expect Ready")
	}

	//Test finalizer
	mco.ObjectMeta.DeletionTimestamp = &v1.Time{Time: time.Now()}
	mco.ObjectMeta.Finalizers = []string{certFinalizer, "test-finalizerr"}
	mco.ObjectMeta.ResourceVersion = updatedMCO.ObjectMeta.ResourceVersion
	err = cl.Update(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
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

func TestCheckObjStorageStatus(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfigObject{
				MetricObjectStorage: &mcov1beta2.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
			},
		},
	}

	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	objs := []runtime.Object{mco}
	c := fake.NewFakeClient(objs...)
	mcoCondition := checkObjStorageStatus(c, mco)
	if mcoCondition == nil {
		t.Errorf("check s3 conf failed: got %v, expected non-nil", mcoCondition)
	}

	err := c.Create(context.TODO(), createSecret("test", "test", mcoconfig.GetDefaultNamespace()))
	if err != nil {
		t.Fatalf("Failed to create secret: (%v)", err)
	}

	mcoCondition = checkObjStorageStatus(c, mco)
	if mcoCondition != nil {
		t.Errorf("check s3 conf failed: got %v, expected nil", mcoCondition)
	}

	updateSecret := createSecret("error", "test", mcoconfig.GetDefaultNamespace())
	updateSecret.ObjectMeta.ResourceVersion = "1"
	err = c.Update(context.TODO(), updateSecret)
	if err != nil {
		t.Fatalf("Failed to update secret: (%v)", err)
	}

	mcoCondition = checkObjStorageStatus(c, mco)
	if mcoCondition == nil {
		t.Errorf("check s3 conf failed: got %v, expected no-nil", mcoCondition)
	}
}

func TestHandleStorageSizeChange(t *testing.T) {
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfigObject{
				MetricObjectStorage: &mcov1beta2.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				AlertmanagerStorageSize: "2Gi",
			},
		},
	}

	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	objs := []runtime.Object{
		mco,
		createStatefulSet(mco.Name, config.GetDefaultNamespace(), "test"),
		createPersistentVolumeClaim(mco.Name, config.GetDefaultNamespace(), "test"),
	}
	c := fake.NewFakeClient(objs...)
	r := &MultiClusterObservabilityReconciler{Client: c, Scheme: s}
	r.HandleStorageSizeChange(mco)

	pvc := &corev1.PersistentVolumeClaim{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      "test",
		Namespace: config.GetDefaultNamespace(),
	}, pvc)

	if err == nil {
		if !pvc.Spec.Resources.Requests.Storage().Equal(resource.MustParse("2Gi")) {
			t.Errorf("update pvc failed: got %v, expected 2Gi", pvc.Spec.Resources.Requests.Storage())
		}
	} else {
		t.Errorf("update pvc failed: %v", err)
	}

}

func createStatefulSet(name, namespace, statefulSetName string) *appsv1.StatefulSet {
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
	}
}

func createPersistentVolumeClaim(name, namespace, pvcName string) *corev1.PersistentVolumeClaim {
	storage := "gp2"
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			Labels: map[string]string{
				"observability.open-cluster-management.io/name": name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storage,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("1Gi"),
				},
			},
		},
	}
}
