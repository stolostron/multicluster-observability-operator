// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package collector_test

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/go-logr/logr"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/collector"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stretchr/testify/assert"
)

const (
	metricsCollectorName    = "metrics-collector-deployment"
	namespace               = "testNamespace"
	uwlMetricsCollectorName = "uwl-metrics-collector-deployment"
	uwlNamespace            = "openshift-user-workload-monitoring"
	uwlSts                  = "prometheus-user-workload"
)

func TestMetricsCollectorResourcesUpdate(t *testing.T) {
	baseMetricsCollector := func() *collector.MetricsCollector {
		return &collector.MetricsCollector{
			// Client is set in each test case
			ClusterInfo: collector.ClusterInfo{
				ClusterID: "test-cluster",
			},
			HubInfo: &operatorconfig.HubInfo{
				ClusterName:              "test-cluster",
				ObservatoriumAPIEndpoint: "http://test-endpoint",
			},
			Log:       logr.Logger{},
			Namespace: namespace,
			ObsAddon: &oav1beta1.ObservabilityAddon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-addon",
					Namespace: namespace,
				},
				Spec: oashared.ObservabilityAddonSpec{
					EnableMetrics: true,
					Interval:      60,
				},
			},
			ServiceAccountName: "test-sa",
		}
	}

	testCases := map[string]struct {
		newMetricsCollector func() *collector.MetricsCollector
		clientObjects       func() []runtime.Object
		request             ctrl.Request
		expects             func(*testing.T, *appsv1.Deployment, *appsv1.Deployment)
	}{
		"Should replicate endpoint operator settings": {
			newMetricsCollector: func() *collector.MetricsCollector {
				return baseMetricsCollector()
			},
			clientObjects: func() []runtime.Object { return []runtime.Object{getEndpointOperatorDeployment(namespace)} },
			expects: func(t *testing.T, deployment, uwlDeployment *appsv1.Deployment) {
				// Check env vars
				operatorEnv := getEndpointOperatorDeployment(namespace).Spec.Template.Spec.Containers[0].Env
				collectorEnv := deployment.Spec.Template.Spec.Containers[0].Env
				if err := checkProxyEnvVars(operatorEnv, collectorEnv); err != nil {
					t.Fatalf("Failed to ensure proxy env vars: %v", err)
				}

				// Check toleration and node selector
				if !slices.Equal(deployment.Spec.Template.Spec.Tolerations, getEndpointOperatorDeployment(namespace).Spec.Template.Spec.Tolerations) {
					t.Fatalf("Tolerations are not set correctly: expected %v, got %v",
						getEndpointOperatorDeployment(namespace).Spec.Template.Spec.Tolerations, deployment.Spec.Template.Spec.Tolerations)
				}
				if !maps.Equal(deployment.Spec.Template.Spec.NodeSelector, getEndpointOperatorDeployment(namespace).Spec.Template.Spec.NodeSelector) {
					t.Fatalf("NodeSelector is not set correctly: expected %v, got %v",
						getEndpointOperatorDeployment(namespace).Spec.Template.Spec.NodeSelector, deployment.Spec.Template.Spec.NodeSelector)
				}

				// Check annotations
				v, ok := deployment.Spec.Template.Annotations[operatorconfig.WorkloadPartitioningPodAnnotationKey]
				if !ok || v != operatorconfig.WorkloadPodExpectedValueJSON {
					t.Fatalf("Failed to find annotation %v: %v on the pod spec of deployment: %v",
						operatorconfig.WorkloadPartitioningPodAnnotationKey,
						operatorconfig.WorkloadPodExpectedValueJSON,
						metricsCollectorName,
					)
				}
			},
		},
		"Should have 0 replicas when metrics is disabled and is not hub collector": {
			newMetricsCollector: func() *collector.MetricsCollector {
				ret := baseMetricsCollector()
				ret.ObsAddon.Spec.EnableMetrics = false
				ret.ClusterInfo.IsHubMetricsCollector = false
				return ret
			},
			clientObjects: func() []runtime.Object { return []runtime.Object{getEndpointOperatorDeployment(namespace)} },
			expects: func(t *testing.T, deployment *appsv1.Deployment, uwlDeployment *appsv1.Deployment) {
				if *deployment.Spec.Replicas != 0 {
					t.Fatalf("Replicas should be 0 when metrics is disabled and is not hub collector")
				}
			},
		},
		"Hub metrics collector should have 1 replica even if metrics is disabled": {
			newMetricsCollector: func() *collector.MetricsCollector {
				ret := baseMetricsCollector()
				ret.ObsAddon.Spec.EnableMetrics = false
				ret.ClusterInfo.IsHubMetricsCollector = true
				return ret
			},
			clientObjects: func() []runtime.Object { return []runtime.Object{getEndpointOperatorDeployment(namespace)} },
			expects: func(t *testing.T, deployment *appsv1.Deployment, uwlDeployment *appsv1.Deployment) {
				if *deployment.Spec.Replicas != 1 {
					t.Fatalf("Hub metrics collector should have 1 replica even if metrics is disabled")
				}
			},
		},
		"Should force reload if certs are updated": {
			newMetricsCollector: func() *collector.MetricsCollector {
				return baseMetricsCollector()
			},
			clientObjects: func() []runtime.Object {
				ret := []runtime.Object{getEndpointOperatorDeployment(namespace)}
				metricsCollector := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      metricsCollectorName,
						Namespace: namespace,
					},
					Spec: appsv1.DeploymentSpec{},
				}
				metricsCollector.Status.ReadyReplicas = 1
				ret = append(ret, metricsCollector)
				return ret
			},
			request: ctrl.Request{NamespacedName: types.NamespacedName{Name: "observability-managed-cluster-certs"}},
			expects: func(t *testing.T, deployment *appsv1.Deployment, uwlDeployment *appsv1.Deployment) {
				if _, ok := deployment.Spec.Template.ObjectMeta.Labels["cert/time-restarted"]; !ok {
					t.Fatalf("Should force reload if certs are updated. Label not found: %v", deployment.Spec.Template.ObjectMeta.Labels)
				}
			},
		},
		"Should create a uwl metrics collector if a custom uwl allowlist is present and uwl prometheus is present": {
			newMetricsCollector: func() *collector.MetricsCollector {
				return baseMetricsCollector()
			},
			clientObjects: func() []runtime.Object {
				data := map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.UwlMetricsConfigMapKey: {
						NameList: []string{"custom_c"},
					},
				}
				uwlAllowlistCM := newAllowListCm(operatorconfig.AllowlistCustomConfigMapName, "default", data)
				ret := []runtime.Object{getEndpointOperatorDeployment(namespace), newUwlPrometheus(), uwlAllowlistCM}
				return ret
			},
			expects: func(t *testing.T, deployment *appsv1.Deployment, uwlDeployment *appsv1.Deployment) {
				if uwlDeployment == nil {
					t.Fatalf("Should create a uwl metrics collector if a custom allowlist is present and uwl prometheus is present")
				}

				command := uwlDeployment.Spec.Template.Spec.Containers[0].Command
				if !slices.Contains(command, `--match={__name__="custom_c",namespace="default"}`) {
					t.Fatalf("Custom allowlist not found in args: %v", command)
				}
			},
		},
		"Should not create a uwl metrics collector if no custom allowlist is present": {
			newMetricsCollector: func() *collector.MetricsCollector {
				return baseMetricsCollector()
			},
			clientObjects: func() []runtime.Object {
				ret := []runtime.Object{getEndpointOperatorDeployment(namespace), newUwlPrometheus()}
				return ret
			},
			expects: func(t *testing.T, deployment *appsv1.Deployment, uwlDeployment *appsv1.Deployment) {
				if uwlDeployment != nil {
					t.Fatalf("Should not create a uwl metrics collector if no custom allowlist is present")
				}
			},
		},
		"Should delete uwl metrics collector if uwl prometheus is removed": {
			newMetricsCollector: func() *collector.MetricsCollector {
				return baseMetricsCollector()
			},
			clientObjects: func() []runtime.Object {
				uwlDeploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uwlMetricsCollectorName,
						Namespace: namespace,
					},
				}
				data := map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.UwlMetricsConfigMapKey: {
						NameList: []string{"custom_c"},
					},
				}
				uwlAllowlistCM := newAllowListCm(operatorconfig.AllowlistCustomConfigMapName, "default", data)
				ret := []runtime.Object{getEndpointOperatorDeployment(namespace), uwlAllowlistCM, uwlDeploy}
				return ret
			},
			expects: func(t *testing.T, deployment *appsv1.Deployment, uwlDeployment *appsv1.Deployment) {
				if uwlDeployment != nil {
					t.Fatalf("Should delete uwl metrics collector if uwl prometheus is removed")
				}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			s := scheme.Scheme
			promv1.AddToScheme(s)
			c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(tc.clientObjects()...).Build()

			metricsCollector := tc.newMetricsCollector()
			metricsCollector.Client = c
			if err := metricsCollector.Update(context.Background(), tc.request); err != nil {
				t.Fatalf("Failed to update metrics collector: %v", err)
			}

			deployment := getMetricsCollectorDeployment(t, context.Background(), c, metricsCollectorName, namespace)
			uwlDeployment := getMetricsCollectorDeployment(t, context.Background(), c, uwlMetricsCollectorName, namespace)
			tc.expects(t, deployment, uwlDeployment)
		})
	}

}

// TestMetricsLists ensures that configured metrics are collected appropriately.
func TestMetricsLists(t *testing.T) {
	addonNamespace := "test-namespace"
	endpointOperatorDeploy := getEndpointOperatorDeployment(addonNamespace)
	s := scheme.Scheme
	promv1.AddToScheme(s)
	testCases := map[string]struct {
		resources               []runtime.Object
		expectMetricsCommand    func(*testing.T, []string)
		expectUwlMetricsCommand func(*testing.T, []string)
	}{
		"Should collect platform metrics": {
			resources: []runtime.Object{
				newAllowListCm(operatorconfig.AllowlistConfigMapName, addonNamespace, map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.MetricsConfigMapKey: {
						NameList:  []string{"a"},
						MatchList: []string{"b"},
					},
				}),
			},
			expectMetricsCommand: func(t *testing.T, commands []string) {
				assert.Contains(t, commands, `--match={__name__="a"}`)
			},
			expectUwlMetricsCommand: func(t *testing.T, commands []string) {
				assert.Len(t, commands, 0)
			},
		},
		"Should collect uwl metrics": {
			resources: []runtime.Object{
				newAllowListCm(operatorconfig.AllowlistConfigMapName, addonNamespace, map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.UwlMetricsConfigMapKey: {
						NameList: []string{"c"},
					},
				}),
				newAllowListCm(operatorconfig.AllowlistCustomConfigMapName, addonNamespace, map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.UwlMetricsConfigMapKey: {
						NameList: []string{"b"},
					},
				}),
			},
			expectMetricsCommand: func(t *testing.T, commands []string) {
				assert.NotContains(t, commands, `--match={__name__="b"}`)
				assert.NotContains(t, commands, `--match={__name__="c"}`)
			},
			expectUwlMetricsCommand: func(t *testing.T, commands []string) {
				assert.Contains(t, commands, `--match={__name__="b"}`)
				assert.Contains(t, commands, `--match={__name__="c"}`)
			},
		},
		"Should set recording rules": {
			resources: []runtime.Object{
				newAllowListCm(operatorconfig.AllowlistConfigMapName, addonNamespace, map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.MetricsConfigMapKey: {
						RecordingRuleList: []operatorconfig.RecordingRule{
							{
								Record: "test_record",
								Expr:   "test_expr",
							},
						},
					},
				}),
				newAllowListCm(operatorconfig.AllowlistCustomConfigMapName, addonNamespace, map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.UwlMetricsConfigMapKey: {
						RecordingRuleList: []operatorconfig.RecordingRule{
							{
								Record: "test_record_b",
								Expr:   "test_expr_b",
							},
						},
					},
				}),
			},
			expectMetricsCommand: func(t *testing.T, commands []string) {
				assert.Contains(t, commands, `--recordingrule={"name":"test_record","query":"test_expr"}`)
				assert.NotContains(t, commands, `--recordingrule={"name":"test_record_b","query":"test_expr_b"}`)
			},
			// expectUwlMetricsCommand: func(t *testing.T, commands []string) {
			// 	assert.NotContains(t, commands, `--recordingrule={"name":"test_record","query":"test_expr"}`)
			// 	assert.Contains(t, commands, `--recordingrule={"name":"test_record_b","query":"test_expr_b"}`)
			// },
		},
		"Should ignore deprecated rules list field": {
			resources: []runtime.Object{
				newAllowListCm(operatorconfig.AllowlistConfigMapName, addonNamespace, map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.MetricsConfigMapKey: {
						RuleList: []operatorconfig.RecordingRule{
							{
								Record: "test_record",
								Expr:   "test_expr",
							},
						},
					},
				}),
				newAllowListCm(operatorconfig.AllowlistCustomConfigMapName, addonNamespace, map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.UwlMetricsConfigMapKey: {
						RuleList: []operatorconfig.RecordingRule{
							{
								Record: "test_record_b",
								Expr:   "test_expr_b",
							},
						},
					},
				}),
			},
			expectMetricsCommand: func(t *testing.T, commands []string) {
				assert.NotContains(t, commands, `--recordingrule={"name":"test_record","query":"test_expr"}`)
				assert.NotContains(t, commands, `--recordingrule={"name":"test_record_b","query":"test_expr_b"}`)
			},
			expectUwlMetricsCommand: func(t *testing.T, commands []string) {
				assert.NotContains(t, commands, `--recordingrule={"name":"test_record","query":"test_expr"}`)
				assert.NotContains(t, commands, `--recordingrule={"name":"test_record_b","query":"test_expr_b"}`)
			},
		},
		"Should set rename map": {
			resources: []runtime.Object{
				newAllowListCm(operatorconfig.AllowlistConfigMapName, addonNamespace, map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.MetricsConfigMapKey: {
						RenameMap: map[string]string{
							"old_name": "new_name",
						},
					},
				}),
				newAllowListCm(operatorconfig.AllowlistCustomConfigMapName, addonNamespace, map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.UwlMetricsConfigMapKey: {
						RenameMap: map[string]string{
							"old_name_b": "new_name_b",
						},
					},
				}),
			},
			expectMetricsCommand: func(t *testing.T, commands []string) {
				assert.Contains(t, commands, `--rename="old_name=new_name"`)
				assert.NotContains(t, commands, `--rename="old_name_b=new_name_b"`)
			},
			// expectUwlMetricsCommand: func(t *testing.T, commands []string) {
			// 	assert.NotContains(t, commands, `--rename={"old_name":"new_name"}`)
			// 	assert.Contains(t, commands, `--rename={"old_name_b":"new_name_b"}`)
			// },
		},
		"Should set collect rule groups": {
			resources: []runtime.Object{
				newAllowListCm(operatorconfig.AllowlistConfigMapName, addonNamespace, map[string]operatorconfig.MetricsAllowlist{
					operatorconfig.MetricsConfigMapKey: {
						CollectRuleGroupList: []operatorconfig.CollectRuleGroup{
							{
								Name: "test_job",
								Annotations: map[string]string{
									"test": "test",
								},
								Selector: operatorconfig.CollectRuleSelector{
									MatchExpression: []metav1.LabelSelectorRequirement{
										{
											Key:      "clusterType",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"sno"},
										},
									},
								},
								CollectRuleList: []operatorconfig.CollectRule{
									{
										Collect: "test_rule",
										Expr:    "test_expr",
										For:     "5m",
										Annotations: map[string]string{
											"test": "test",
										},
										Metrics: operatorconfig.DynamicMetrics{
											NameList:  []string{"a"},
											MatchList: []string{"b"},
										},
									},
								},
							},
							{
								Name: "test_job",
								Annotations: map[string]string{
									"test": "test",
								},
								Selector: operatorconfig.CollectRuleSelector{
									MatchExpression: []metav1.LabelSelectorRequirement{
										{
											Key:      "clusterType",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{"not-sno"},
										},
									},
								},
								CollectRuleList: []operatorconfig.CollectRule{
									{
										Collect: "not_sno_rule",
										Expr:    "test_expr",
										For:     "5m",
										Annotations: map[string]string{
											"test": "test",
										},
										Metrics: operatorconfig.DynamicMetrics{
											NameList:  []string{"a"},
											MatchList: []string{"b"},
										},
									},
								},
							},
						},
					},
				}),
			},
			expectMetricsCommand: func(t *testing.T, commands []string) {
				assert.Contains(t, commands, `--collectrule={"name":"test_rule","expr":"test_expr","for":"5m","names":["a"],"matches":["b"]}`)
				assert.NotContains(t, commands, `--collectrule={"name":"not_sno_rule","expr":"test_expr","for":"5m","names":["a"],"matches":["b"]}`)
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			resources := []runtime.Object{endpointOperatorDeploy, newPrometheusUwlResources()}
			resources = append(resources, tc.resources...)
			c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(resources...).Build()

			metricsCollector := &collector.MetricsCollector{
				Client: c,
				ClusterInfo: collector.ClusterInfo{
					ClusterID:   "test-cluster",
					ClusterType: "sno",
				},
				HubInfo: &operatorconfig.HubInfo{
					ClusterName:              "mycluster",
					ObservatoriumAPIEndpoint: "http://test-endpoint",
				},
				Log:       logr.Logger{},
				Namespace: addonNamespace,
				ObsAddon: &oav1beta1.ObservabilityAddon{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "observability-addon",
						Namespace: addonNamespace,
					},
					Spec: oashared.ObservabilityAddonSpec{},
				},
				ServiceAccountName: "test-sa",
			}
			if err := metricsCollector.Update(context.Background(), ctrl.Request{}); err != nil {
				t.Fatalf("Failed to update metrics collector: %v", err)
			}

			// Get metrics-collector and uwl metrics-collector deployments
			metricsColDeploy := getMetricsCollectorDeployment(t, context.Background(), c, metricsCollectorName, addonNamespace)
			metricsColCommands := metricsColDeploy.Spec.Template.Spec.Containers[0].Command
			uwlMetricsColDeploy := getMetricsCollectorDeployment(t, context.Background(), c, uwlMetricsCollectorName, addonNamespace)
			uwlMetricsColCommands := []string{}
			if uwlMetricsColDeploy != nil {
				uwlMetricsColCommands = uwlMetricsColDeploy.Spec.Template.Spec.Containers[0].Command
			}

			if tc.expectMetricsCommand != nil {
				tc.expectMetricsCommand(t, metricsColCommands)
			}

			if tc.expectUwlMetricsCommand != nil {
				tc.expectUwlMetricsCommand(t, uwlMetricsColCommands)
			}
		})
	}

}

func getEndpointOperatorDeployment(ns string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "endpoint-observability-operator",
			Namespace: ns,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "endpoint-observability-operator",
							Env: []corev1.EnvVar{
								{
									Name:  "HTTP_PROXY",
									Value: "http://foo.com",
								},
								{
									Name:  "HTTPS_PROXY",
									Value: "https://foo.com",
								},
								{
									Name:  "NO_PROXY",
									Value: "bar.com",
								},
								{
									Name:  "HTTPS_PROXY_CA_BUNDLE",
									Value: "custom-ca.crt",
								},
							},
						},
					},
				},
			},
		},
	}
}

func getMetricsCollectorDeployment(t *testing.T, ctx context.Context, c client.Client, name, ns string) *appsv1.Deployment {
	deployment := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		t.Fatalf("Failed to get deployment %s/%s: %v", namespace, name, err)
	}
	return deployment
}

func newUwlPrometheus() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uwlSts,
			Namespace: uwlNamespace,
		},
	}
}

func newAllowListCm(name, namespace string, data map[string]operatorconfig.MetricsAllowlist) *corev1.ConfigMap {
	cmData := make(map[string]string, len(data))
	for k, v := range data {
		strData, err := yaml.Marshal(v)
		if err != nil {
			panic(err)
		}
		cmData[k] = string(strData)
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: cmData,
	}
}

func newPrometheusUwlResources() client.Object {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus-user-workload",
			Namespace: "openshift-user-workload-monitoring",
		},
		Spec: appsv1.StatefulSetSpec{
			// Replicas: util.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "prometheus-user-workload",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "prometheus-user-workload",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "prometheus-user-workload",
							Image: "test",
						},
					},
				},
			},
		},
	}
}

func checkProxyEnvVars(expect, has []corev1.EnvVar) error {
	toCompare := map[string]string{"HTTP_PROXY": "", "HTTPS_PROXY": "", "NO_PROXY": "", "HTTPS_PROXY_CA_BUNDLE": ""}
	expectMap := make(map[string]string, len(toCompare))
	for _, e := range expect {
		if _, ok := toCompare[e.Name]; ok {
			if len(e.Value) == 0 {
				return fmt.Errorf("Env var %s is empty in the expected list", e.Name)
			}
			expectMap[e.Name] = e.Value
		}
	}

	if len(expect) != len(toCompare) {
		return fmt.Errorf("Some env vars are missing in the expected list: expected %v, got %v", toCompare, expect)
	}

	hasMap := make(map[string]string, len(toCompare))
	for _, e := range has {
		if v, ok := expectMap[e.Name]; ok {
			if v != e.Value {
				return fmt.Errorf("Env var %s is not set correctly: expected %s, got %s", e.Name, v, e.Value)
			}
			hasMap[e.Name] = e.Value
		}
	}

	if len(hasMap) != len(toCompare) {
		return fmt.Errorf("Some env vars are missing in the actual list: expected %v, got %v", toCompare, hasMap)
	}

	return nil
}
