// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package hypershift

import (
	"context"
	"fmt"
	"reflect"

	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AcmEtcdSmName      = "acm-etcd"
	AcmApiServerSmName = "acm-kube-apiserver"
	EtcdSmName         = "etcd"
	ApiServerSmName    = "kube-apiserver"
)

// ReconcileHostedClustersServiceMonitors reconciles ServiceMonitors for hypershift hosted clusters
// It returns the number of hosted clusters reconciled and an error if any
// For each hosted cluster, it adds a ServiceMonitor for etcd and kube-apiserver
// so that metrics are scraped by the user workload cluster's prometheus
// with relevant labels added for ACM.
// No watcher is set on hypershift CRDs. Adding the CRDs after the controller is started
// will not trigger the reconcile function. The controller must be restarted to watch the new CRDs.
func ReconcileHostedClustersServiceMonitors(ctx context.Context, c client.Client) (int, error) {
	hostedClusters := &hyperv1.HostedClusterList{}
	if err := c.List(ctx, hostedClusters, &client.ListOptions{}); err != nil {
		return 0, fmt.Errorf("failed to list HostedClusterList: %w", err)
	}

	reconciledHCsCount := 0
	for _, cluster := range hostedClusters.Items {
		namespace := fmt.Sprintf("%s-%s", cluster.ObjectMeta.Namespace, cluster.ObjectMeta.Name)

		etcdSMDesired, err := getEtcdServiceMonitor(ctx, c, namespace, cluster.Spec.ClusterID, cluster.ObjectMeta.Name)
		if err != nil {
			return reconciledHCsCount, fmt.Errorf("failed to get etcd ServiceMonitor: %w", err)
		}

		if err := createOrUpdateSM(ctx, c, etcdSMDesired); err != nil {
			return reconciledHCsCount, fmt.Errorf("failed to create/update etcd ServiceMonitor %s/%s: %w", etcdSMDesired.GetNamespace(), etcdSMDesired.GetName(), err)
		}

		apiServerSMDesired, err := getKubeServiceMonitor(ctx, c, namespace, cluster.Spec.ClusterID, cluster.ObjectMeta.Name)
		if err != nil {
			return reconciledHCsCount, fmt.Errorf("failed to get kube-apiserver ServiceMonitor: %w", err)
		}

		if err := createOrUpdateSM(ctx, c, apiServerSMDesired); err != nil {
			return reconciledHCsCount, fmt.Errorf("failed to create/update api-server ServiceMonitor %s/%s: %w", apiServerSMDesired.GetNamespace(), apiServerSMDesired.GetName(), err)
		}

		reconciledHCsCount++
	}

	return reconciledHCsCount, nil
}

// DeleteServiceMonitors deletes ACM ServiceMonitors for all hosted clusters
func DeleteServiceMonitors(ctx context.Context, c client.Client) error {
	hList := &hyperv1.HostedClusterList{}
	if err := c.List(ctx, hList, &client.ListOptions{}); err != nil {
		return fmt.Errorf("failed to list HyperShiftDeployment: %w", err)
	}

	for _, cluster := range hList.Items {
		namespace := HostedClusterNamespace(&cluster) // nolint:gosec
		if err := deleteServiceMonitor(ctx, c, AcmEtcdSmName, namespace); err != nil {
			return fmt.Errorf("failed to delete ServiceMonitor %s/%s: %w", namespace, AcmEtcdSmName, err)
		}

		if err := deleteServiceMonitor(ctx, c, AcmApiServerSmName, namespace); err != nil {
			return fmt.Errorf("failed to delete ServiceMonitor %s/%s: %w", namespace, AcmApiServerSmName, err)
		}
	}

	return nil
}

// HostedClusterNamespace returns the namespace for a hosted cluster
// This is a helper function to centralize the logic for creating the namespace name
func HostedClusterNamespace(cluster *hyperv1.HostedCluster) string {
	return fmt.Sprintf("%s-%s", cluster.ObjectMeta.Namespace, cluster.ObjectMeta.Name)
}

func createOrUpdateSM(ctx context.Context, c client.Client, smDesired *promv1.ServiceMonitor) error {
	smCurrent := &promv1.ServiceMonitor{}
	if err := c.Get(ctx, types.NamespacedName{Name: smDesired.GetName(), Namespace: smDesired.GetNamespace()}, smCurrent); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get ServiceMonitor: %w", err)
		}

		if err := c.Create(ctx, smDesired, &client.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create ServiceMonitor: %w", err)
		}
		return nil
	}

	if reflect.DeepEqual(smCurrent.Spec, smDesired.Spec) {
		return nil
	}

	smDesired.ObjectMeta.ResourceVersion = smCurrent.ObjectMeta.ResourceVersion
	if err := c.Update(ctx, smDesired, &client.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update ServiceMonitor: %w", err)
	}

	return nil
}

func getEtcdServiceMonitor(ctx context.Context, c client.Client, namespace, clusterID, clusterName string) (*promv1.ServiceMonitor, error) {
	// Get the hypershift's etcd service monitor to replicate some of its settings
	hypershiftEtcdSM := &promv1.ServiceMonitor{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "etcd"}, hypershiftEtcdSM); err != nil {
		return nil, fmt.Errorf("failed to get hypershift's etcd ServiceMonitor: %w", err)
	}

	if len(hypershiftEtcdSM.Spec.Endpoints) != 1 {
		return nil, fmt.Errorf("hypershift's etcd ServiceMonitor has more than one endpoint") // safe check
	}

	originalEndpoint := hypershiftEtcdSM.Spec.Endpoints[0]

	return &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AcmEtcdSmName,
			Namespace: namespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{
				{
					Scheme:            "https",
					Interval:          "15s",
					Port:              originalEndpoint.Port,
					BearerTokenSecret: corev1.SecretKeySelector{},
					TLSConfig:         originalEndpoint.TLSConfig,
					MetricRelabelConfigs: []*promv1.RelabelConfig{
						{
							SourceLabels: []string{"__name__"},
							Action:       "keep",
							Regex: "(etcd_server_has_leader|etcd_disk_wal_fsync_duration_seconds_bucket|" +
								"etcd_mvcc_db_total_size_in_bytes|etcd_network_peer_round_trip_time_seconds_bucket|" +
								"etcd_mvcc_db_total_size_in_use_in_bytes|" +
								"etcd_disk_backend_commit_duration_seconds_bucket|" +
								"etcd_server_leader_changes_seen_total)",
						},
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: clusterID,
						},
						{
							TargetLabel: "cluster_id",
							Action:      "replace",
							Replacement: clusterID,
						},
						{
							TargetLabel: "cluster",
							Action:      "replace",
							Replacement: clusterName,
						},
					},
					RelabelConfigs: []*promv1.RelabelConfig{
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: clusterID,
						},
						{
							TargetLabel: "job",
							Action:      "replace",
							Replacement: "etcd",
						},
					},
				},
			},
			Selector:          hypershiftEtcdSM.Spec.Selector,
			NamespaceSelector: hypershiftEtcdSM.Spec.NamespaceSelector,
		},
	}, nil
}

func getKubeServiceMonitor(ctx context.Context, c client.Client, namespace, clusterID, clusterName string) (*promv1.ServiceMonitor, error) {
	// Get the hypershift's api-server service monitor and replicate some of its settings
	hypershiftKubeSM := &promv1.ServiceMonitor{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ApiServerSmName}, hypershiftKubeSM); err != nil {
		return nil, fmt.Errorf("failed to get hypershift's kube-apiserver ServiceMonitor: %w", err)
	}

	smEndpointsLen := len(hypershiftKubeSM.Spec.Endpoints)
	if smEndpointsLen != 1 {
		return nil, fmt.Errorf("expecting one endpoint from hypershift's kube-apiserver ServiceMonitor, has %d", smEndpointsLen) // safe check
	}

	originalEndpoint := hypershiftKubeSM.Spec.Endpoints[0]

	return &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AcmApiServerSmName,
			Namespace: namespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{
				{
					Scheme:            "https",
					Interval:          "15s",
					TargetPort:        originalEndpoint.TargetPort,
					BearerTokenSecret: corev1.SecretKeySelector{},
					TLSConfig:         originalEndpoint.TLSConfig,
					MetricRelabelConfigs: []*promv1.RelabelConfig{
						{
							SourceLabels: []string{"__name__"},
							Action:       "keep",
							Regex: "(up|apiserver_request_duration_seconds_bucket|apiserver_storage_objects|" +
								"apiserver_request_total|apiserver_current_inflight_requests)",
						},
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: clusterID,
						},
						{
							TargetLabel: "cluster_id",
							Action:      "replace",
							Replacement: clusterID,
						},
						{
							TargetLabel: "cluster",
							Action:      "replace",
							Replacement: clusterName,
						},
					},
					RelabelConfigs: []*promv1.RelabelConfig{
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: clusterID,
						},
						{
							TargetLabel: "job",
							Action:      "replace",
							Replacement: "apiserver",
						},
					},
				},
			},
			Selector:          hypershiftKubeSM.Spec.Selector,
			NamespaceSelector: hypershiftKubeSM.Spec.NamespaceSelector,
		},
	}, nil
}

func deleteServiceMonitor(ctx context.Context, c client.Client, name, namespace string) error {
	sm := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	if err := c.Delete(ctx, sm); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete ServiceMonitor: %w", err)
		}
	}

	return nil
}
