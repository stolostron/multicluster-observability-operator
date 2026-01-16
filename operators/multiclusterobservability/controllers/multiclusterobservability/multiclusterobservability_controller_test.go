// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	oauthv1 "github.com/openshift/api/oauth/v1"
	routev1 "github.com/openshift/api/route/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	observatoriumv1alpha1 "github.com/stolostron/observatorium-operator/api/v1alpha1"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/util/workqueue"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	migrationv1alpha1 "sigs.k8s.io/kube-storage-version-migrator/pkg/apis/migration/v1alpha1"

	imagev1 "github.com/openshift/api/image/v1"
	fakeimageclient "github.com/openshift/client-go/image/clientset/versioned/fake"
	fakeimagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1/fake"
)

func init() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stdout)))
	os.Setenv("UNIT_TEST", "true")
}

func setupTest(t *testing.T) func() {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get work dir: (%v)", err)
	}
	t.Log("begin setupTest")
	os.MkdirAll(path.Join(wd, "../../tests"), 0o755)
	testManifestsPath := path.Join(wd, "../../tests/manifests")
	manifestsPath := path.Join(wd, "../../manifests")
	os.Setenv("TEMPLATES_PATH", testManifestsPath)
	templates.ResetTemplates()
	// clean up the manifest path if left over from previous test
	if fi, err := os.Lstat(testManifestsPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if err = os.Remove(testManifestsPath); err != nil {
			t.Logf("Failed to delete symlink(%s) for the test manifests: (%v)", testManifestsPath, err)
		}
	}
	err = os.Symlink(manifestsPath, testManifestsPath)
	if err != nil {
		t.Fatalf("Failed to create symbollink(%s) to(%s) for the test manifests: (%v)", testManifestsPath, manifestsPath, err)
	}
	t.Log("setupTest done")

	return func() {
		t.Log("begin teardownTest")
		if err = os.Remove(testManifestsPath); err != nil {
			t.Logf("Failed to delete symbollink(%s) for the test manifests: (%v)", testManifestsPath, err)
		}
		os.Remove(path.Join(wd, "../../tests"))
		os.Unsetenv("TEMPLATES_PATH")
		t.Log("teardownTest done")
	}
}

func newTestCert(name string, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("test-ca-crt"),
			"tls.crt": []byte("test-tls-crt"),
			"tls.key": []byte("test-tls-key"),
		},
	}
}

var testImagemanifestsMap = map[string]string{
	"endpoint_monitoring_operator":     "test.io/endpoint-monitoring:test",
	"grafana":                          "test.io/origin-grafana:test",
	"grafana_dashboard_loader":         "test.io/grafana-dashboard-loader:test",
	"management_ingress":               "test.io/management-ingress:test",
	"observatorium":                    "test.io/observatorium:test",
	"observatorium_operator":           "test.io/observatorium-operator:test",
	"prometheus_alertmanager":          "test.io/prometheus-alertmanager:test",
	"configmap_reloader":               "test.io/configmap-reloader:test",
	"rbac_query_proxy":                 "test.io/rbac-query-proxy:test",
	"thanos":                           "test.io/thanos:test",
	"thanos_receive_controller":        "test.io/thanos_receive_controller:test",
	"kube_rbac_proxy":                  "test.io/kube-rbac-proxy:test",
	"multicluster_observability_addon": "test.io/multicluster-observability-addon:test",
}

func newTestImageManifestsConfigMap(namespace, version string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ImageManifestConfigMapNamePrefix + version,
			Namespace: namespace,
			Labels: map[string]string{
				config.OCMManifestConfigMapTypeLabelKey:    config.OCMManifestConfigMapTypeLabelValue,
				config.OCMManifestConfigMapVersionLabelKey: version,
			},
		},
		Data: testImagemanifestsMap,
	}
}

func newMCHInstanceWithVersion(namespace, version string) *mchv1.MultiClusterHub {
	return &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: namespace,
		},
		Spec: mchv1.MultiClusterHubSpec{},
		Status: mchv1.MultiClusterHubStatus{
			CurrentVersion: version,
			DesiredVersion: version,
		},
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
			Replicas:      1,
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
			Replicas:          1,
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

func TestMultiClusterMonitoringCRUpdate(t *testing.T) {
	var (
		name      = "monitoring"
		namespace = config.GetDefaultNamespace()
	)

	defer setupTest(t)()
	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				config.AnnotationKeyImageTagSuffix: "tag",
			},
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
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
			ObservabilityAddonSpec: &mcoshared.ObservabilityAddonSpec{
				EnableMetrics: false,
			},
		},
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	oav1beta1.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)
	routev1.AddToScheme(s)
	oauthv1.AddToScheme(s)
	clusterv1.AddToScheme(s)
	policyv1.AddToScheme(s)
	addonv1alpha1.AddToScheme(s)
	migrationv1alpha1.SchemeBuilder.AddToScheme(s)

	svc := createObservatoriumAPIService(name, namespace)
	serverCACerts := newTestCert(config.ServerCACerts, namespace)
	clientCACerts := newTestCert(config.ClientCACerts, namespace)
	grafanaCert := newTestCert(config.GrafanaCerts, namespace)
	serverCert := newTestCert(config.ServerCerts, namespace)
	// byo case for proxy
	proxyRouteBYOCACerts := newTestCert(config.ProxyRouteBYOCAName, namespace)
	proxyRouteBYOCert := newTestCert(config.ProxyRouteBYOCERTName, namespace)
	// byo case for the alertmanager route
	testAmRouteBYOCaSecret := newTestCert(config.AlertmanagerRouteBYOCAName, namespace)
	testAmRouteBYOCertSecret := newTestCert(config.AlertmanagerRouteBYOCERTName, namespace)
	clustermgmtAddon := newClusterManagementAddon()
	extensionApiserverAuthenticationCM := &corev1.ConfigMap{ // required by alertmanager
		ObjectMeta: metav1.ObjectMeta{
			Name:      "extension-apiserver-authentication",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"client-ca-file": "test",
		},
	}

	objs := []runtime.Object{
		mco, svc, serverCACerts, clientCACerts, proxyRouteBYOCACerts, grafanaCert, serverCert,
		testAmRouteBYOCaSecret, testAmRouteBYOCertSecret, proxyRouteBYOCert, clustermgmtAddon, extensionApiserverAuthenticationCM,
	}
	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().
		WithRuntimeObjects(objs...).
		WithStatusSubresource(
			&addonv1alpha1.ManagedClusterAddOn{},
			&mcov1beta2.MultiClusterObservability{},
			&oav1beta1.ObservabilityAddon{},
		).
		Build()

		// Create fake imagestream client
	imageClient := &fakeimagev1client.FakeImageV1{Fake: &(fakeimageclient.NewSimpleClientset().Fake)}
	_, err := imageClient.ImageStreams(config.OauthProxyImageStreamNamespace).Create(context.Background(),
		&imagev1.ImageStream{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.OauthProxyImageStreamName,
				Namespace: config.OauthProxyImageStreamNamespace,
			},
			Spec: imagev1.ImageStreamSpec{
				Tags: []imagev1.TagReference{
					{
						Name: "v4.4",
						From: &corev1.ObjectReference{
							Kind: "DockerImage",
							Name: "quay.io/openshift-release-dev/ocp-v4.0-art-dev",
						},
					},
				},
			},
		}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Create a ReconcileMemcached object with the scheme and fake client.
	r := &MultiClusterObservabilityReconciler{Client: cl, Scheme: s, CRDMap: map[string]bool{config.IngressControllerCRD: true}, ImageClient: imageClient}
	config.SetMonitoringCRName(name)
	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: name,
		},
	}

	// Create empty client. The test secret specified in MCO is not yet created.
	t.Log("Reconcile empty client")
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	// wait for update status
	time.Sleep(1 * time.Second)

	// verify openshiftcluster monitoring label is set to true in namespace
	updatedNS := &corev1.Namespace{}
	err = cl.Get(context.TODO(), types.NamespacedName{
		Name: namespace,
	}, updatedNS)
	if err != nil {
		t.Fatalf("Failed to get namespace: (%v)", err)
	}
	if val, ok := updatedNS.ObjectMeta.Labels[config.OpenShiftClusterMonitoringlabel]; !ok || val != "true" {
		t.Fatalf("Failed to get correct namespace label, expect true")
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

	amRoute := &routev1.Route{}
	err = cl.Get(context.TODO(), types.NamespacedName{
		Name:      config.AlertmanagerRouteName,
		Namespace: namespace,
	}, amRoute)
	if err != nil {
		t.Fatalf("Failed to get alertmanager's route: (%v)", err)
	}
	// check the BYO certificate for alertmanager's route
	if amRoute.Spec.TLS.CACertificate != "test-tls-crt" ||
		amRoute.Spec.TLS.Certificate != "test-tls-crt" ||
		amRoute.Spec.TLS.Key != "test-tls-key" {
		t.Fatalf("incorrect certificate for alertmanager's route")
	}

	err = cl.Create(context.TODO(), createSecret("test", "test", namespace))
	if err != nil {
		t.Fatalf("Failed to create secret: (%v)", err)
	}

	// backup label test for Secret
	req2 := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: namespace,
		},
	}

	t.Log("---- Reconcile secret, verify backup label ---- ")
	_, err = r.Reconcile(context.TODO(), req2)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	// wait for update status
	time.Sleep(1 * time.Second)

	updatedObjectStoreSecret := &corev1.Secret{}
	err = r.Client.Get(context.TODO(), req2.NamespacedName, updatedObjectStoreSecret)
	if err != nil {
		t.Fatalf("backup Failed to get ObjectStore secret (%v)", err)
	}

	if _, ok := updatedObjectStoreSecret.Labels[config.BackupLabelName]; !ok {
		t.Fatalf("Missing backup label on: (%v)", updatedObjectStoreSecret)
	}

	// backup label test for Configmap
	err = cl.Create(context.TODO(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertRuleCustomConfigMapName,
			Namespace: namespace,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create configmap: (%v)", err)
	}

	req2 = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      config.AlertRuleCustomConfigMapName,
			Namespace: namespace,
		},
	}

	t.Log("---- Reconcile configmap, verify backup label ---- ")
	_, err = r.Reconcile(context.TODO(), req2)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	// wait for update status
	time.Sleep(1 * time.Second)

	updatedConfigmap := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), req2.NamespacedName, updatedConfigmap)
	if err != nil {
		t.Fatalf("backup Failed to get configmap (%v)", err)
	}

	if _, ok := updatedConfigmap.Labels[config.BackupLabelName]; !ok {
		t.Fatalf("Missing backup label on: (%v)", updatedConfigmap)
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
	expectedDeploymentNames := getExpectedDeploymentNames()
	for _, deployName := range expectedDeploymentNames {
		deploy := createReadyDeployment(deployName, namespace)
		err = cl.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, deploy)
		if errors.IsNotFound(err) {
			t.Log(err)
			err = cl.Create(context.TODO(), deploy)
			if err != nil {
				t.Fatalf("Failed to create deployment %s: %v", deployName, err)
			}
		}
	}

	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	// wait for update status
	time.Sleep(1 * time.Second)

	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}
	status = findStatusCondition(updatedMCO.Status.Conditions, "Failed")
	if status == nil || status.Reason != "StatefulSetNotFound" {
		t.Errorf("Failed to get correct MCO status, expect Failed")
	}

	expectedStatefulSetNames := getExpectedStatefulSetNames()
	for _, statefulName := range expectedStatefulSetNames {
		deploy := createReadyStatefulSet(name, namespace, statefulName)
		err = cl.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, deploy)
		if errors.IsNotFound(err) {
			err = cl.Create(context.TODO(), deploy)
			if err != nil {
				t.Fatalf("Failed to create stateful set %s: %v", statefulName, err)
			}
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
	// wait for update status
	time.Sleep(1 * time.Second)

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
	// reconcile to make sure the finalizer of the mco cr is deleted
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	// wait for the stop status update channel is closed
	time.Sleep(1 * time.Second)

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
	// wait for update status
	time.Sleep(1 * time.Second)

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
	err = cl.Delete(context.TODO(), createReadyStatefulSet(
		name,
		namespace,
		config.GetOperandNamePrefix()+"alertmanager"))
	if err != nil {
		t.Fatalf("Failed to delete alertmanager: (%v)", err)
	}
	failedAlertManager := createFailedStatefulSet(
		name,
		namespace,
		config.GetOperandNamePrefix()+"alertmanager")
	err = cl.Create(context.TODO(), failedAlertManager)
	if err != nil {
		t.Fatalf("Failed to create alertmanager: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	// wait for update status
	time.Sleep(1 * time.Second)

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
	err = cl.Delete(context.TODO(), createReadyDeployment(config.GetOperandNamePrefix()+"rbac-query-proxy", namespace))
	if err != nil {
		t.Fatalf("Failed to delete rbac-query-proxy: (%v)", err)
	}
	err = cl.Delete(context.TODO(), failedAlertManager)
	if err != nil {
		t.Fatalf("Failed to delete alertmanager: (%v)", err)
	}
	err = cl.Create(context.TODO(), createReadyStatefulSet(
		name,
		namespace,
		config.GetOperandNamePrefix()+"alertmanager"))
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
	// wait for update status
	time.Sleep(1 * time.Second)

	updatedMCO = &mcov1beta2.MultiClusterObservability{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, updatedMCO)
	if err != nil {
		t.Fatalf("Failed to get MultiClusterObservability: (%v)", err)
	}

	status = findStatusCondition(updatedMCO.Status.Conditions, "Ready")
	if status == nil || status.Reason != "Ready" {
		t.Errorf("Failed to get correct MCO status, expect Ready")
	}

	// Test finalizer
	mco.ObjectMeta.Finalizers = []string{resFinalizer, "test-finalizerr"}
	mco.ObjectMeta.ResourceVersion = updatedMCO.ObjectMeta.ResourceVersion
	err = cl.Update(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to update MultiClusterObservability: (%v)", err)
	}
	err = cl.Delete(context.TODO(), mco)
	if err != nil {
		t.Fatalf("Failed to delete MultiClusterObservability: (%v)", err)
	}
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile for finalizer: (%v)", err)
	}
}

func TestImageReplaceForMCO(t *testing.T) {
	var (
		name      = "test-monitoring"
		namespace = config.GetDefaultNamespace()
		version   = "2.3.0"
	)

	defer setupTest(t)()

	// A MultiClusterObservability object with metadata and spec.
	mco := &mcov1beta2.MultiClusterObservability{
		TypeMeta: metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
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
			ObservabilityAddonSpec: &mcoshared.ObservabilityAddonSpec{
				EnableMetrics: false,
			},
		},
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	observatoriumv1alpha1.AddToScheme(s)
	routev1.AddToScheme(s)
	oauthv1.AddToScheme(s)
	clusterv1.AddToScheme(s)
	policyv1.AddToScheme(s)
	addonv1alpha1.AddToScheme(s)
	mchv1.SchemeBuilder.AddToScheme(s)
	migrationv1alpha1.SchemeBuilder.AddToScheme(s)

	observatoriumAPIsvc := createObservatoriumAPIService(name, namespace)
	serverCACerts := newTestCert(config.ServerCACerts, namespace)
	clientCACerts := newTestCert(config.ClientCACerts, namespace)
	grafanaCert := newTestCert(config.GrafanaCerts, namespace)
	serverCert := newTestCert(config.ServerCerts, namespace)
	// create the image manifest configmap
	testMCHInstance := newMCHInstanceWithVersion(config.GetMCONamespace(), version)
	imageManifestsCM := newTestImageManifestsConfigMap(config.GetMCONamespace(), version)
	// byo case for the alertmanager route
	testAmRouteBYOCaSecret := newTestCert(config.AlertmanagerRouteBYOCAName, namespace)
	testAmRouteBYOCertSecret := newTestCert(config.AlertmanagerRouteBYOCERTName, namespace)
	clustermgmtAddon := newClusterManagementAddon()
	extensionApiserverAuthenticationCM := &corev1.ConfigMap{ // required by alertmanager
		ObjectMeta: metav1.ObjectMeta{
			Name:      "extension-apiserver-authentication",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"client-ca-file": "test",
		},
	}

	objs := []runtime.Object{
		mco, observatoriumAPIsvc, serverCACerts, clientCACerts, grafanaCert, serverCert,
		testMCHInstance, imageManifestsCM, testAmRouteBYOCaSecret, testAmRouteBYOCertSecret, clustermgmtAddon, extensionApiserverAuthenticationCM,
	}
	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

	// Create fake imagestream client
	imageClient := &fakeimagev1client.FakeImageV1{Fake: &(fakeimageclient.NewSimpleClientset().Fake)}
	_, err := imageClient.ImageStreams(config.OauthProxyImageStreamNamespace).Create(context.Background(),
		&imagev1.ImageStream{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.OauthProxyImageStreamName,
				Namespace: config.OauthProxyImageStreamNamespace,
			},
			Spec: imagev1.ImageStreamSpec{
				Tags: []imagev1.TagReference{
					{
						Name: "v4.4",
						From: &corev1.ObjectReference{
							Kind: "DockerImage",
							Name: "quay.io/openshift-release-dev/ocp-v4.0-art-dev",
						},
					},
				},
			},
		}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Create a ReconcileMemcached object with the scheme and fake client.
	r := &MultiClusterObservabilityReconciler{Client: cl, Scheme: s, CRDMap: map[string]bool{config.MCHCrdName: true, config.IngressControllerCRD: true}, ImageClient: imageClient}
	config.SetMonitoringCRName(name)

	// Mock request to simulate Reconcile() being called on an event for a watched resource .
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      config.MCHUpdatedRequestName,
			Namespace: config.GetMCONamespace(),
		},
	}

	// set the image manifests map for testing
	config.SetImageManifests(testImagemanifestsMap)

	// trigger another reconcile for MCH update event
	_, err = r.Reconcile(context.TODO(), req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	// wait for update status
	time.Sleep(1 * time.Second)

	expectedDeploymentNames := []string{
		config.GetOperandNamePrefix() + config.Grafana,
		config.GetOperandNamePrefix() + config.ObservatoriumOperator,
		config.GetOperandNamePrefix() + config.RBACQueryProxy,
	}
	for _, deployName := range expectedDeploymentNames {
		deploy := &appsv1.Deployment{}
		err = cl.Get(context.TODO(), types.NamespacedName{
			Name:      deployName,
			Namespace: namespace,
		}, deploy)
		if err != nil {
			t.Fatalf("Failed to get deployment %s: %v", deployName, err)
		}
		for _, container := range deploy.Spec.Template.Spec.Containers {
			imageKey := strings.ReplaceAll(container.Name, "-", "_")
			switch container.Name {
			case "oauth-proxy":
				// TODO: add oauth-proxy image to image manifests
				continue
			case "grafana-proxy":
				continue
			case "config-reloader":
				imageKey = "configmap_reloader"
			}
			imageValue, exists := testImagemanifestsMap[imageKey]
			if !exists {
				t.Fatalf("The image key(%s) for the container(%s) doesn't exist in the deployment(%s)", imageKey, container.Name, deployName)
			}
			if imageValue != container.Image {
				t.Fatalf("The image(%s) for the container(%s) in the deployment(%s) should be replaced with the one(%s) in the image manifests", container.Image, container.Name, deployName, imageValue)
			}
		}
	}

	expectedStatefulSetNames := []string{
		config.GetOperandNamePrefix() + config.Alertmanager,
	}
	for _, statefulName := range expectedStatefulSetNames {
		sts := &appsv1.StatefulSet{}
		err = cl.Get(context.TODO(), types.NamespacedName{
			Name:      statefulName,
			Namespace: namespace,
		}, sts)
		if err != nil {
			t.Fatalf("Failed to get statefulset %s: %v", statefulName, err)
		}
		for _, container := range sts.Spec.Template.Spec.Containers {
			imageKey := strings.ReplaceAll(container.Name, "-", "_")
			switch container.Name {
			case "oauth-proxy", "alertmanager-proxy":
				// TODO: add oauth-proxy image to image manifests
				continue
			case "alertmanager":
				imageKey = "prometheus_alertmanager"
			case "config-reloader":
				imageKey = "configmap_reloader"
			}
			imageValue, exists := testImagemanifestsMap[imageKey]
			if !exists {
				t.Fatalf("The image key(%s) for the container(%s) doesn't exist in the statefulset(%s)", imageKey, container.Name, statefulName)
			}
			if imageValue != container.Image {
				t.Fatalf("The image(%s) for the container(%s) in the statefulset(%s) should not replace with the one in the image manifests", imageValue, container.Name, statefulName)
			}
		}
	}

	// stop update status routine
	stopStatusUpdate <- struct{}{}
	// wait for update status
	time.Sleep(1 * time.Second)
}

func createSecret(key, name, namespace string) *corev1.Secret {
	s3Conf := &config.ObjectStorgeConf{
		Type: "s3",
		Config: config.Config{
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
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
			},
		},
	}

	s := scheme.Scheme
	mcov1beta2.SchemeBuilder.AddToScheme(s)
	objs := []runtime.Object{mco}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	mcoCondition := checkObjStorageStatus(c, mco)
	if mcoCondition == nil {
		t.Errorf("check s3 conf failed: got %v, expected non-nil", mcoCondition)
	}

	err := c.Create(context.TODO(), createSecret("test", "test", config.GetDefaultNamespace()))
	if err != nil {
		t.Fatalf("Failed to create secret: (%v)", err)
	}

	mcoCondition = checkObjStorageStatus(c, mco)
	if mcoCondition != nil {
		t.Errorf("check s3 conf failed: got %v, expected nil", mcoCondition)
	}

	updateSecret := createSecret("error", "test", config.GetDefaultNamespace())
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
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
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
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	r := &MultiClusterObservabilityReconciler{Client: c, Scheme: s}
	isAlertmanagerStorageSizeChanged = true
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
				"alertmanager": "observability",
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
				"alertmanager": "observability",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storage,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
}

func newMultiClusterObservability() *mcov1beta2.MultiClusterObservability {
	return &mcov1beta2.MultiClusterObservability{
		TypeMeta:   metav1.TypeMeta{Kind: "MultiClusterObservability"},
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: mcov1beta2.MultiClusterObservabilitySpec{
			StorageConfig: &mcov1beta2.StorageConfig{
				MetricObjectStorage: &mcoshared.PreConfiguredStorage{
					Key:  "test",
					Name: "test",
				},
				AlertmanagerStorageSize: "2Gi",
			},
		},
	}
}

func createNamespaceInstance(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"openshift.io/cluster-monitoring": "true",
			},
		},
	}
}

func createAlertManagerConfigMap(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: config.GetDefaultNamespace(),
		},
	}
}

func TestPrometheusRulesRemovedFromOpenshiftMonitoringNamespace(t *testing.T) {
	promRule := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "acm-observability-alert-rules",
			Namespace: "openshift-monitoring",
		},
		// Sample rules
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name: "test",
					Rules: []monitoringv1.Rule{
						{
							Alert: "test",
						},
					},
				},
			},
		},
	}
	s := scheme.Scheme
	monitoringv1.AddToScheme(s)
	objs := []runtime.Object{promRule}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	r := &MultiClusterObservabilityReconciler{Client: c, Scheme: s}
	err := r.deleteSpecificPrometheusRule(context.TODO())
	if err != nil {
		t.Fatalf("Failed to delete PrometheusRule: (%v)", err)
	}
}

func TestServiceMonitorRemovedFromOpenshiftMonitoringNamespace(t *testing.T) {
	sm := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observability-sm-test",
			Namespace: "openshift-monitoring",
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: []monitoringv1.Endpoint{
				{
					Port: "test",
				},
			},
		},
	}
	s := scheme.Scheme
	monitoringv1.AddToScheme(s)
	objs := []runtime.Object{sm}
	c := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
	r := &MultiClusterObservabilityReconciler{Client: c, Scheme: s}
	err := r.deleteServiceMonitorInOpenshiftMonitoringNamespace(context.TODO())
	if err != nil {
		t.Fatalf("Failed to delete ServiceMonitor: (%v)", err)
	}
}

func TestNewMCOACRDEventHandler(t *testing.T) {
	// Register the necessary schemes
	scheme := runtime.NewScheme()
	mcov1beta2.AddToScheme(scheme)

	existingObjs := []runtime.Object{
		&mcov1beta2.MultiClusterObservability{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-mco",
				Namespace: "default",
			},
		},
	}

	tests := []struct {
		name         string
		crdName      string
		expectedReqs []reconcile.Request
	}{
		{
			name:         "CRD created is not a dependency",
			crdName:      "non-supported-crd",
			expectedReqs: []reconcile.Request{},
		},
		{
			name:    "CRD created is a dependency",
			crdName: "clusterlogforwarders.observability.openshift.io",
			expectedReqs: []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      "test-mco",
						Namespace: "default",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existingObjs...).Build()
			handler := newMCOACRDEventHandler(client)

			obj := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.crdName,
				},
			}

			createEvent := event.CreateEvent{
				Object: obj,
			}

			// Create a workqueue
			queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
			handler.Create(context.TODO(), createEvent, queue)

			reqs := []reconcile.Request{}
			for queue.Len() > 0 {
				item, _ := queue.Get()
				reqs = append(reqs, item)
				queue.Done(item)
			}

			assert.Equal(t, tt.expectedReqs, reqs)
		})
	}
}
