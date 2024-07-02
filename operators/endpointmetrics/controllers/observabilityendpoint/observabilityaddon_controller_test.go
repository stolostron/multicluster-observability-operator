// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"context"
	"os"
	"strings"
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"golang.org/x/exp/slices"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/openshift"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

const (
	name         = "observability-addon"
	restartLabel = "cert/time-restarted"
)

var (
	cv = &ocinfrav1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Spec: ocinfrav1.ClusterVersionSpec{
			ClusterID: testClusterID,
		},
	}
	infra = &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Status: ocinfrav1.InfrastructureStatus{
			ControlPlaneTopology: ocinfrav1.SingleReplicaTopologyMode,
		},
	}
)

func newObservabilityAddon(name string, ns string) *oav1beta1.ObservabilityAddon {
	return &oav1beta1.ObservabilityAddon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: oashared.ObservabilityAddonSpec{
			EnableMetrics: true,
			Interval:      60,
		},
	}
}

func newPromSvc() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      promSvcName,
			Namespace: promNamespace,
		},
	}
}

func newHubInfoSecret(data []byte, ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.HubInfoSecretName,
			Namespace: ns,
		},
		Data: map[string][]byte{
			operatorconfig.HubInfoSecretKey: data,
			operatorconfig.ClusterNameKey:   []byte("test-cluster"),
		},
	}
}

func newAMAccessorSecret(ns string, val string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hubAmAccessorSecretName,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"token": []byte(val),
		},
	}
}

func newClusterMonitoringConfigCM(configDataStr string, mgr string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterMonitoringConfigName,
			Namespace: promNamespace,
			ManagedFields: []metav1.ManagedFieldsEntry{
				{
					Manager:   mgr,
					Operation: metav1.ManagedFieldsOperationUpdate,
				},
			},
		},
		Data: map[string]string{
			clusterMonitoringConfigDataKey: configDataStr,
		},
	}
}

func newImagesCM(ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.ImageConfigMap,
			Namespace: ns,
		},
		Data: map[string]string{
			operatorconfig.MetricsCollectorKey:   "metrics-collector-image",
			operatorconfig.NodeExporterKey:       "node-exporter-image",
			operatorconfig.KubeStateMetricsKey:   "kube-state-metrics-image",
			operatorconfig.KubeRbacProxyKey:      "kube-rbac-proxy-image",
			operatorconfig.PrometheusOperatorKey: "prometheus-operator-image",
		},
	}
}

func init() {
	os.Setenv("UNIT_TEST", "true")
}

func TestObservabilityAddonController(t *testing.T) {
	hubInfoData := []byte(`
endpoint: "http://test-endpoint"
alertmanager-endpoint: "http://test-alertamanger-endpoint"
alertmanager-router-ca: |
    -----BEGIN CERTIFICATE-----
    xxxxxxxxxxxxxxxxxxxxxxxxxxx
    -----END CERTIFICATE-----
`)

	testNamespace := "test-ns"
	testHubNamespace := "test-hub-ns"
	hubObjs := []runtime.Object{}
	hubInfo := newHubInfoSecret(hubInfoData, testNamespace)
	amAccessSrt := newAMAccessorSecret(testNamespace, "test-token")
	allowList := getAllowlistCM(testNamespace)
	images := newImagesCM(testNamespace)
	objs := []runtime.Object{hubInfo, amAccessSrt, allowList, images, cv, infra,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "extension-apiserver-authentication",
				Namespace: "kube-system",
			},
			Data: map[string]string{
				"client-ca-file": "test",
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-ns",
			},
		},
		&appv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "endpoint-observability-operator",
				Namespace: "test-ns",
			},
		},
	}

	s := runtime.NewScheme()
	kubescheme.AddToScheme(s)
	addonv1alpha1.AddToScheme(s)
	oav1beta1.AddToScheme(s)
	ocinfrav1.AddToScheme(s)
	hyperv1.AddToScheme(s)
	promv1.AddToScheme(s)

	hubClient := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(hubObjs...).Build()
	c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objs...).Build()

	hubClientWithReload, err := util.NewReloadableHubClientWithReloadFunc(func() (client.Client, error) {
		return hubClient, nil
	})
	if err != nil {
		t.Fatalf("Failed to create hub client with reload: %v", err)
	}
	r := &ObservabilityAddonReconciler{
		Client:                c,
		HubClient:             hubClientWithReload,
		Scheme:                s,
		IsHubMetricsCollector: false,
		Namespace:             testNamespace,
		HubNamespace:          testHubNamespace,
		ServiceAccountName:    "test-sa",
	}

	// test error in reconcile if missing obervabilityaddon
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "install",
			Namespace: testNamespace,
		},
	}
	ctx := context.TODO()
	_, err = r.Reconcile(ctx, req)
	if err == nil {
		t.Fatalf("reconcile: miss the error for missing obervabilityaddon")
	}

	// test reconcile w/o prometheus-k8s svc
	err = hubClient.Create(ctx, newObservabilityAddon(name, testHubNamespace))
	if err != nil {
		t.Fatalf("failed to create hub oba to install: (%v)", err)
	}
	oba := newObservabilityAddon(name, testNamespace)
	err = c.Create(ctx, oba)
	if err != nil {
		t.Fatalf("failed to create oba to install: (%v)", err)
	}
	req = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "install",
			Namespace: testNamespace,
		},
	}
	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}

	// test reconcile successfully with all resources installed and finalizer set
	promSvc := newPromSvc()
	err = c.Create(ctx, promSvc)
	if err != nil {
		t.Fatalf("failed to create prom svc to install: (%v)", err)
	}
	req = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "install",
			Namespace: testNamespace,
		},
	}
	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	rb := &rbacv1.ClusterRoleBinding{}
	err = c.Get(ctx, types.NamespacedName{Name: openshift.ClusterRoleBindingName,
		Namespace: ""}, rb)
	if err != nil {
		t.Fatalf("Required clusterrolebinding not created: (%v)", err)
	}
	cm := &corev1.ConfigMap{}
	err = c.Get(ctx, types.NamespacedName{Name: openshift.CaConfigmapName,
		Namespace: testNamespace}, cm)
	if err != nil {
		t.Fatalf("Required configmap not created: (%v)", err)
	}
	deploy := &appv1.Deployment{}
	err = c.Get(ctx, types.NamespacedName{Name: metricsCollectorName,
		Namespace: testNamespace}, deploy)
	if err != nil {
		t.Fatalf("Metrics collector deployment not created: (%v)", err)
	}
	foundOba := &oav1beta1.ObservabilityAddon{}
	err = hubClient.Get(ctx, types.NamespacedName{Name: obAddonName,
		Namespace: testHubNamespace}, foundOba)
	if err != nil {
		t.Fatalf("Failed to get observabilityAddon: (%v)", err)
	}
	if !slices.Contains(foundOba.Finalizers, obsAddonFinalizer) {
		t.Fatal("Finalizer not set in observabilityAddon")
	}

	// test reconcile w/o clusterversion(OCP 3.11)
	c.Delete(ctx, cv)
	req = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "install",
			Namespace: testNamespace,
		},
	}
	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	err = c.Get(ctx, types.NamespacedName{Name: metricsCollectorName,
		Namespace: testNamespace}, deploy)
	if err != nil {
		t.Fatalf("Metrics collector deployment not created: (%v)", err)
	}
	commands := deploy.Spec.Template.Spec.Containers[0].Command
	for _, cmd := range commands {
		if strings.Contains(cmd, "clusterID=") && !strings.Contains(cmd, "test-cluster") {
			t.Fatalf("Found wrong clusterID in command: (%s)", cmd)
		}
	}

	// test reconcile metrics collector deployment updated if cert secret updated
	found := &appv1.Deployment{}
	err = c.Get(ctx, types.NamespacedName{Name: metricsCollectorName,
		Namespace: testNamespace}, found)
	if err != nil {
		t.Fatalf("Metrics collector deployment not found: (%v)", err)
	}
	found.Status.ReadyReplicas = 1
	err = c.Status().Update(ctx, found)
	if err != nil {
		t.Fatalf("Failed to update metrics collector deployment: (%v)", err)
	}
	req = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      mtlsCertName,
			Namespace: testNamespace,
		},
	}
	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile for update: (%v)", err)
	}
	err = c.Get(ctx, types.NamespacedName{Name: metricsCollectorName,
		Namespace: testNamespace}, deploy)
	if err != nil {
		t.Fatalf("Metrics collector deployment not found: (%v)", err)
	}
	if deploy.Spec.Template.ObjectMeta.Labels[restartLabel] == "" {
		t.Fatal("Deployment not updated")
	}

	// test reconcile  metrics collector's replicas set to 0 if observability disabled
	err = c.Delete(ctx, oba)
	if err != nil {
		t.Fatalf("failed to delete obsaddon to disable: (%v)", err)
	}
	oba = newObservabilityAddon(name, testNamespace)
	oba.Spec.EnableMetrics = false
	err = c.Create(ctx, oba)
	if err != nil {
		t.Fatalf("failed to create obsaddon to disable: (%v)", err)
	}
	req = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "disable",
			Namespace: testNamespace,
		},
	}
	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile for disable: (%v)", err)
	}
	err = c.Get(ctx, types.NamespacedName{Name: metricsCollectorName,
		Namespace: testNamespace}, deploy)
	if err != nil {
		t.Fatalf("Metrics collector deployment not created: (%v)", err)
	}
	if *deploy.Spec.Replicas != 0 {
		t.Fatalf("Replicas for metrics collector deployment is not set as 0, value is (%d)", *deploy.Spec.Replicas)
	}

	// test reconcile all resources and finalizer are removed
	err = c.Delete(ctx, oba)
	if err != nil {
		t.Fatalf("failed to delete obsaddon to delete: (%v)", err)
	}
	req = ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "delete",
			Namespace: testNamespace,
		},
	}
	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile for delete: (%v)", err)
	}
	err = c.Get(ctx, types.NamespacedName{Name: openshift.ClusterRoleBindingName,
		Namespace: ""}, rb)
	if !errors.IsNotFound(err) {
		t.Fatalf("Required clusterrolebinding not deleted")
	}
	err = c.Get(ctx, types.NamespacedName{Name: openshift.CaConfigmapName,
		Namespace: testNamespace}, cm)
	if !errors.IsNotFound(err) {
		t.Fatalf("Required configmap not deleted")
	}
	err = c.Get(ctx, types.NamespacedName{Name: metricsCollectorName,
		Namespace: testNamespace}, deploy)
	if !errors.IsNotFound(err) {
		t.Fatalf("Metrics collector deployment not deleted")
	}
	foundOba1 := &oav1beta1.ObservabilityAddon{}
	err = hubClient.Get(ctx, types.NamespacedName{Name: obAddonName,
		Namespace: testHubNamespace}, foundOba1)
	if err != nil {
		t.Fatalf("Failed to get observabilityAddon: (%v)", err)
	}
	if slices.Contains(foundOba1.Finalizers, obsAddonFinalizer) {
		t.Fatal("Finalizer not removed from observabilityAddon")
	}
}

func getAllowlistCM(ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.AllowlistConfigMapName,
			Namespace: ns,
		},
		Data: map[string]string{
			operatorconfig.MetricsConfigMapKey: `
names:
  - a
  - b
matches:
  - __name__="c"
recording_rules:
  - record: f
    expr: g
collect_rules:
  - name: h
    selector:
      matchExpressions:
        - key: clusterType
          operator: NotIn
          values: ["SNO"]
    rules:
      - collect: j
        expr: k
        for: 1m
        names:
          - c
        matches:
          - __name__="a"
`,
			operatorconfig.UwlMetricsConfigMapKey: `
names:
  - uwl_a
  - uwl_b
`},
	}
}
