// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package hypershift_test

import (
	"context"
	"fmt"
	"testing"

	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/hypershift"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHypershiftServiceMonitors(t *testing.T) {
	hostedClusterName := "test-hosted-cluster"
	hosteClusterNamespace := "clusters"
	hCluster := &hyperv1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hostedClusterName,
			Namespace: hosteClusterNamespace,
		},
		Spec: hyperv1.HostedClusterSpec{
			ClusterID: "test-hosted-cluster-id",
		},
	}

	etcdSm := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hypershift.AcmEtcdSmName,
			Namespace: hypershift.HostedClusterNamespace(hCluster),
		},
		Spec: promv1.ServiceMonitorSpec{},
	}

	hypershiftEtcdSM := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hypershift.EtcdSmName,
			Namespace: hypershift.HostedClusterNamespace(hCluster),
		},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{
				{
					Port: "metrics",
					TLSConfig: &promv1.TLSConfig{
						SafeTLSConfig: promv1.SafeTLSConfig{
							InsecureSkipVerify: true,
						},
					},
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"component": "etcd",
				},
			},
			NamespaceSelector: promv1.NamespaceSelector{
				MatchNames: []string{hosteClusterNamespace},
			},
		},
	}

	hypershiftApiServerSM := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hypershift.ApiServerSmName,
			Namespace: hypershift.HostedClusterNamespace(hCluster),
		},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{
				{
					Port: "metrics",
					TLSConfig: &promv1.TLSConfig{
						SafeTLSConfig: promv1.SafeTLSConfig{
							CA: promv1.SecretOrConfigMap{
								Secret: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "kube-apiserver-cert",
									},
									Key: "ca.crt",
								},
							},
						},
					},
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"component": "etcd",
				},
			},
			NamespaceSelector: promv1.NamespaceSelector{
				MatchNames: []string{hosteClusterNamespace},
			},
		},
	}

	scheme := runtime.NewScheme()
	hyperv1.AddToScheme(scheme)
	promv1.AddToScheme(scheme)

	testCases := map[string]struct {
		getClient   func() client.Client
		expectError bool
		expectSMs   bool
	}{
		"no hyperfhift cluster": {
			getClient:   func() client.Client { return fake.NewClientBuilder().WithScheme(scheme).Build() },
			expectError: false,
			expectSMs:   false,
		},
		"original hypershift sm is missing": {
			getClient: func() client.Client {
				return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(hCluster).Build()
			},
			expectError: true,
			expectSMs:   false,
		},
		"create": {
			getClient: func() client.Client {
				return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(hCluster, hypershiftEtcdSM, hypershiftApiServerSM).Build()
			},
			expectError: false,
			expectSMs:   true,
		},
		"update": {
			getClient: func() client.Client {
				return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(hCluster, etcdSm, hypershiftEtcdSM, hypershiftApiServerSM).Build()
			},
			expectError: false,
			expectSMs:   true,
		},
	}

	checkRelabelConfigs := func(t *testing.T, sm *promv1.ServiceMonitor) {
		for _, relabelCfg := range sm.Spec.Endpoints[0].MetricRelabelConfigs {
			switch relabelCfg.TargetLabel {
			case "_id":
				assert.Equal(t, "test-hosted-cluster-id", relabelCfg.Replacement)
			case "cluster_id":
				assert.Equal(t, "test-hosted-cluster-id", relabelCfg.Replacement)
			case "cluster":
				assert.Equal(t, "test-hosted-cluster", relabelCfg.Replacement)
			case "":
				continue
			default:
				t.Errorf("unexpected relabel config: %v", relabelCfg)
			}
		}
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			client := tc.getClient()
			hostedClustersReconciled, err := hypershift.ReconcileHostedClustersServiceMonitors(context.Background(), client)
			if tc.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			if !tc.expectSMs {
				assert.Equal(t, 0, hostedClustersReconciled, "expected 0 hosted cluster reconciled")
				return
			}

			assert.Equal(t, 1, hostedClustersReconciled, "expected 1 hosted cluster reconciled")

			// check etcd and kube-apiserver ServiceMonitors are created
			sm := &promv1.ServiceMonitor{}
			err = client.Get(context.Background(), types.NamespacedName{
				Name:      hypershift.AcmEtcdSmName,
				Namespace: fmt.Sprintf("%s-%s", hosteClusterNamespace, hostedClusterName),
			}, sm)
			assert.NoError(t, err)
			assert.Equal(t, hypershiftEtcdSM.Spec.Endpoints[0].TLSConfig, sm.Spec.Endpoints[0].TLSConfig)
			assert.Equal(t, hypershiftEtcdSM.Spec.Selector, sm.Spec.Selector)
			assert.Equal(t, hypershiftEtcdSM.Spec.NamespaceSelector, sm.Spec.NamespaceSelector)
			checkRelabelConfigs(t, sm)

			err = client.Get(context.Background(), types.NamespacedName{
				Name:      hypershift.AcmApiServerSmName,
				Namespace: fmt.Sprintf("%s-%s", hosteClusterNamespace, hostedClusterName),
			}, sm)
			assert.NoError(t, err)
			assert.Equal(t, hypershiftApiServerSM.Spec.Endpoints[0].TLSConfig, sm.Spec.Endpoints[0].TLSConfig)
			assert.Equal(t, hypershiftApiServerSM.Spec.Selector, sm.Spec.Selector)
			assert.Equal(t, hypershiftApiServerSM.Spec.NamespaceSelector, sm.Spec.NamespaceSelector)
			checkRelabelConfigs(t, sm)
		})
	}
}

func TestDeleteServiceMonitors(t *testing.T) {
	hostedClusterName := "test-hosted-cluster"
	hosteClusterNamespace := "clusters"
	hCluster := &hyperv1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hostedClusterName,
			Namespace: hosteClusterNamespace,
		},
		Spec: hyperv1.HostedClusterSpec{
			ClusterID: "test-hosted-cluster-id",
		},
	}

	apiServerSM := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hypershift.ApiServerSmName,
			Namespace: fmt.Sprintf("%s-%s", hosteClusterNamespace, hostedClusterName),
		},
		Spec: promv1.ServiceMonitorSpec{},
	}

	acmApiServerSM := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hypershift.AcmApiServerSmName,
			Namespace: fmt.Sprintf("%s-%s", hosteClusterNamespace, hostedClusterName),
		},
		Spec: promv1.ServiceMonitorSpec{},
	}

	etcdSM := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hypershift.EtcdSmName,
			Namespace: fmt.Sprintf("%s-%s", hosteClusterNamespace, hostedClusterName),
		},
		Spec: promv1.ServiceMonitorSpec{},
	}

	acmEtcdSM := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hypershift.AcmEtcdSmName,
			Namespace: fmt.Sprintf("%s-%s", hosteClusterNamespace, hostedClusterName),
		},
		Spec: promv1.ServiceMonitorSpec{},
	}

	scheme := runtime.NewScheme()
	hyperv1.AddToScheme(scheme)
	promv1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(hCluster, apiServerSM, acmApiServerSM, etcdSM, acmEtcdSM).Build()

	err := hypershift.DeleteServiceMonitors(context.Background(), c)
	assert.NoError(t, err)

	// check if ACM ServiceMonitors are deleted and original ones preserved
	deployedSM := &promv1.ServiceMonitorList{}
	err = c.List(context.Background(), deployedSM, &client.ListOptions{
		Namespace: fmt.Sprintf("%s-%s", hosteClusterNamespace, hostedClusterName),
	})
	assert.NoError(t, err)
	assert.Len(t, deployedSM.Items, 2, "expected 2 hypershift original ServiceMonitors to not be deleted")

	for _, sm := range deployedSM.Items {
		assert.Contains(t, []string{hypershift.ApiServerSmName, hypershift.EtcdSmName}, sm.Name)
	}
}
