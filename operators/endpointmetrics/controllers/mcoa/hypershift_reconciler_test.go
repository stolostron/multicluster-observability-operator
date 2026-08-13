// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	prometheusv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/hypershift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileHyperShift(t *testing.T) {
	t.Setenv("UNIT_TEST", "true")

	scheme := runtime.NewScheme()
	require.NoError(t, hyperv1.AddToScheme(scheme))
	require.NoError(t, promv1.AddToScheme(scheme))
	require.NoError(t, prometheusv1alpha1.AddToScheme(scheme))

	namespace := "addon-ns"
	hcpNamespace := "clusters-myhc"

	hostedCluster := &hyperv1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "myhc", Namespace: "clusters"},
		Spec:       hyperv1.HostedClusterSpec{ClusterID: "hcp-cluster-id"},
	}
	targetPort := intstr.FromString("metrics")
	hypershiftEtcdSM := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{Name: hypershift.EtcdSmName, Namespace: hcpNamespace},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{{Port: "metrics", TargetPort: &targetPort}},
			Selector:  *metav1.SetAsLabelSelector(map[string]string{"k8s-app": "etcd"}),
		},
	}
	hypershiftApiServerSM := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{Name: hypershift.ApiServerSmName, Namespace: hcpNamespace},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{{Port: "client", TargetPort: &targetPort}},
			Selector:  *metav1.SetAsLabelSelector(map[string]string{"k8s-app": "apiserver"}),
		},
	}
	etcdScrapeConfig := &prometheusv1alpha1.ScrapeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "etcd-hcp-uwl-metrics", Namespace: namespace,
			Labels: map[string]string{labelKeyComponent: etcdHcpComponentLabel},
		},
		Spec: prometheusv1alpha1.ScrapeConfigSpec{
			Params: map[string][]string{"match[]": {`{__name__="etcd_mvcc_db_total_size_in_bytes"}`}},
		},
	}
	apiserverScrapeConfig := &prometheusv1alpha1.ScrapeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "apiserver-hcp-uwl-metrics", Namespace: namespace,
			Labels: map[string]string{labelKeyComponent: apiserverHcpComponentLabel},
		},
		Spec: prometheusv1alpha1.ScrapeConfigSpec{
			Params: map[string][]string{"match[]": {`{__name__="grpc_server_handled_total"}`}},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(
		hostedCluster, hypershiftEtcdSM, hypershiftApiServerSM, etcdScrapeConfig, apiserverScrapeConfig,
	).Build()

	r := NewMCOAAgentReconciler(
		c, logr.Discard(), scheme, events.NewFakeRecorder(10),
		namespace, "spoke-id", "spoke-name",
		"", "", "", "", true, true,
	)

	require.NoError(t, r.ReconcileHyperShift(context.Background()))

	acmEtcdSM := &promv1.ServiceMonitor{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: hcpNamespace, Name: hypershift.AcmEtcdSmName}, acmEtcdSM))
	assert.Equal(t, "hcp-cluster-id", *acmEtcdSM.Spec.Endpoints[0].MetricRelabelConfigs[1].Replacement)

	acmApiServerSM := &promv1.ServiceMonitor{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: hcpNamespace, Name: hypershift.AcmApiServerSmName}, acmApiServerSM))
	assert.Equal(t, "spoke-id", *acmApiServerSM.Spec.Endpoints[0].MetricRelabelConfigs[3].Replacement)

	updatedEtcdSC := &prometheusv1alpha1.ScrapeConfig{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: etcdScrapeConfig.Name}, updatedEtcdSC))
	require.Len(t, updatedEtcdSC.Spec.MetricRelabelConfigs, 2)
	assert.Equal(t, "keep", updatedEtcdSC.Spec.MetricRelabelConfigs[0].Action)
	assert.Equal(t, promv1.LabelName(clusterIDMetricLabel), updatedEtcdSC.Spec.MetricRelabelConfigs[0].SourceLabels[0])
	assert.Equal(t, ".+", updatedEtcdSC.Spec.MetricRelabelConfigs[0].Regex)
	assert.Equal(t, promv1.LabelName(managementClusterIDMetricLabel), updatedEtcdSC.Spec.MetricRelabelConfigs[1].SourceLabels[0])
	assert.Equal(t, "spoke-id", updatedEtcdSC.Spec.MetricRelabelConfigs[1].Regex)
}

func TestReconcileHostedClustersServiceMonitorsFromMCOAConfig_Nominal(t *testing.T) {
	t.Setenv("UNIT_TEST", "true")

	scheme := runtime.NewScheme()
	require.NoError(t, hyperv1.AddToScheme(scheme))
	require.NoError(t, promv1.AddToScheme(scheme))
	require.NoError(t, prometheusv1alpha1.AddToScheme(scheme))

	namespace := "addon-ns"
	hcpNamespace := "clusters-myhc"

	hostedCluster := &hyperv1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myhc",
			Namespace: "clusters",
		},
		Spec: hyperv1.HostedClusterSpec{
			ClusterID: "hcp-cluster-id",
		},
	}

	targetPort := intstr.FromString("metrics")
	hypershiftEtcdSM := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hypershift.EtcdSmName,
			Namespace: hcpNamespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{{Port: "metrics", TargetPort: &targetPort}},
			Selector:  *metav1.SetAsLabelSelector(map[string]string{"k8s-app": "etcd"}),
		},
	}
	hypershiftApiServerSM := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hypershift.ApiServerSmName,
			Namespace: hcpNamespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{{Port: "client", TargetPort: &targetPort}},
			Selector:  *metav1.SetAsLabelSelector(map[string]string{"k8s-app": "apiserver"}),
		},
	}

	etcdScrapeConfig := &prometheusv1alpha1.ScrapeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-hcp-uwl-metrics",
			Namespace: namespace,
			Labels: map[string]string{
				labelKeyComponent: etcdHcpComponentLabel,
			},
		},
		Spec: prometheusv1alpha1.ScrapeConfigSpec{
			Params: map[string][]string{
				"match[]": {`{__name__="etcd_mvcc_db_total_size_in_bytes"}`},
			},
		},
	}
	apiserverScrapeConfig := &prometheusv1alpha1.ScrapeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apiserver-hcp-uwl-metrics",
			Namespace: namespace,
			Labels: map[string]string{
				labelKeyComponent: apiserverHcpComponentLabel,
			},
		},
		Spec: prometheusv1alpha1.ScrapeConfigSpec{
			Params: map[string][]string{
				"match[]": {`{__name__="grpc_server_handled_total"}`},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(
		hostedCluster,
		hypershiftEtcdSM,
		hypershiftApiServerSM,
		etcdScrapeConfig,
		apiserverScrapeConfig,
	).Build()

	r := NewMCOAAgentReconciler(
		c, logr.Discard(), scheme, events.NewFakeRecorder(10),
		namespace, "spoke-id", "spoke-name",
		"", "", "", "", true, true,
	)

	require.NoError(t, r.ReconcileHyperShift(context.Background()))

	acmEtcdSM := &promv1.ServiceMonitor{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: hcpNamespace, Name: hypershift.AcmEtcdSmName}, acmEtcdSM))
	assert.Contains(t, acmEtcdSM.Spec.Endpoints[0].MetricRelabelConfigs[0].Regex, "etcd_mvcc_db_total_size_in_bytes")
	assert.Equal(t, "hcp-cluster-id", *acmEtcdSM.Spec.Endpoints[0].MetricRelabelConfigs[1].Replacement)
	assert.Equal(t, "spoke-id", *acmEtcdSM.Spec.Endpoints[0].MetricRelabelConfigs[3].Replacement)

	acmApiServerSM := &promv1.ServiceMonitor{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: hcpNamespace, Name: hypershift.AcmApiServerSmName}, acmApiServerSM))
	assert.Contains(t, acmApiServerSM.Spec.Endpoints[0].MetricRelabelConfigs[0].Regex, "grpc_server_handled_total")
}

func TestReconcileHostedClustersServiceMonitorsFromMCOAConfig_NoHypershiftSMs(t *testing.T) {
	t.Setenv("UNIT_TEST", "true")

	scheme := runtime.NewScheme()
	require.NoError(t, hyperv1.AddToScheme(scheme))
	require.NoError(t, promv1.AddToScheme(scheme))
	require.NoError(t, prometheusv1alpha1.AddToScheme(scheme))

	namespace := "addon-ns"
	hostedCluster := &hyperv1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "myhc", Namespace: "clusters"},
		Spec:       hyperv1.HostedClusterSpec{ClusterID: "hcp-cluster-id"},
	}
	etcdScrapeConfig := &prometheusv1alpha1.ScrapeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-hcp-uwl-metrics",
			Namespace: namespace,
			Labels:    map[string]string{labelKeyComponent: etcdHcpComponentLabel},
		},
		Spec: prometheusv1alpha1.ScrapeConfigSpec{
			Params: map[string][]string{"match[]": {`{__name__="etcd_mvcc_db_total_size_in_bytes"}`}},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(hostedCluster, etcdScrapeConfig).Build()

	r := NewMCOAAgentReconciler(
		c, logr.Discard(), scheme, events.NewFakeRecorder(10),
		namespace, "spoke-id", "spoke-name",
		"", "", "", "", true, true,
	)

	require.NoError(t, r.ReconcileHyperShift(context.Background()))

	acmEtcdSM := &promv1.ServiceMonitor{}
	err := c.Get(context.Background(), types.NamespacedName{Namespace: "clusters-myhc", Name: hypershift.AcmEtcdSmName}, acmEtcdSM)
	require.Error(t, err)
}

func TestExtractDependentMetrics(t *testing.T) {
	r := NewMCOAAgentReconciler(
		nil, logr.Discard(), nil, events.NewFakeRecorder(10),
		"", "", "",
		"", "", "", "", true, true,
	)

	testCases := map[string]struct {
		scrapeConfig []prometheusv1alpha1.ScrapeConfig
		rules        []promv1.PrometheusRule
		expectResult []string
		expectError  bool
	}{
		"none": {},
		"invalid scrape config": {
			scrapeConfig: []prometheusv1alpha1.ScrapeConfig{{
				Spec: prometheusv1alpha1.ScrapeConfigSpec{
					Params: map[string][]string{"match[]": {`{__name__"acm_label_names"}`}},
				},
			}},
			expectError: true,
		},
		"scrape config": {
			scrapeConfig: []prometheusv1alpha1.ScrapeConfig{{
				Spec: prometheusv1alpha1.ScrapeConfigSpec{
					Params: map[string][]string{
						"match[]": {
							`{__name__=":node_memory_MemAvailable_bytes:sum"}`,
							`{__name__=~"acm_"}`,
							`{__name__="acm_managed_cluster_labels"}`,
						},
					},
				},
			}},
			expectResult: []string{"acm_managed_cluster_labels"},
		},
		"rule": {
			rules: []promv1.PrometheusRule{{
				Spec: promv1.PrometheusRuleSpec{
					Groups: []promv1.RuleGroup{{
						Rules: []promv1.Rule{{
							Expr: intstr.FromString(`sum(grpc_server_started_total{job="etcd",clusterID!=""})`),
						}},
					}},
				},
			}},
			expectResult: []string{"grpc_server_started_total"},
		},
		"merged scrape config and rule": {
			scrapeConfig: []prometheusv1alpha1.ScrapeConfig{{
				Spec: prometheusv1alpha1.ScrapeConfigSpec{
					Params: map[string][]string{
						"match[]": {
							`{__name__="grpc_server_started_total"}`,
							`{__name__="apiserver_request_duration_seconds_bucket"}`,
						},
					},
				},
			}},
			rules: []promv1.PrometheusRule{{
				Spec: promv1.PrometheusRuleSpec{
					Groups: []promv1.RuleGroup{{
						Rules: []promv1.Rule{{
							Expr: intstr.FromString(`sum(grpc_server_handled_total{job="etcd",clusterID!=""})`),
						}},
					}},
				},
			}},
			expectResult: []string{
				"apiserver_request_duration_seconds_bucket",
				"grpc_server_handled_total",
				"grpc_server_started_total",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			res, err := r.extractDependentMetrics(tc.scrapeConfig, tc.rules)
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expectResult, res)
		})
	}
}

func TestIsHypershiftReconcileRequest(t *testing.T) {
	t.Parallel()

	assert.True(t, isHypershiftReconcileRequest(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: hypershiftReconcileTriggerName},
	}))
	assert.False(t, isHypershiftReconcileRequest(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: hypershift.EtcdSmName, Namespace: "clusters-myhc"},
	}))
	assert.False(t, isHypershiftReconcileRequest(ctrl.Request{
		NamespacedName: types.NamespacedName{Name: hypershift.AcmEtcdSmName, Namespace: "clusters-myhc"},
	}))
}
