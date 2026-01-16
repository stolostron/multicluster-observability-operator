// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package hypershift

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	operatorutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
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
		namespace := fmt.Sprintf("%s-%s", cluster.Namespace, cluster.Name)

		etcdSMDesired, err := getEtcdServiceMonitor(ctx, c, namespace, cluster.Spec.ClusterID, cluster.Name)
		if err != nil {
			return reconciledHCsCount, fmt.Errorf("failed to get etcd ServiceMonitor: %w", err)
		}

		if err := createOrUpdateSM(ctx, c, etcdSMDesired); err != nil {
			return reconciledHCsCount, fmt.Errorf("failed to create/update etcd ServiceMonitor %s/%s: %w", etcdSMDesired.GetNamespace(), etcdSMDesired.GetName(), err)
		}

		apiServerSMDesired, err := getKubeServiceMonitor(ctx, c, namespace, cluster.Spec.ClusterID, cluster.Name)
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
		namespace := HostedClusterNamespace(&cluster)
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
	return fmt.Sprintf("%s-%s", cluster.Namespace, cluster.Name)
}

func IsHypershiftCluster(ctx context.Context) (bool, error) {
	var isHypershift bool
	crdClient, err := operatorutil.GetOrCreateCRDClient()
	if err != nil {
		return false, fmt.Errorf("failed to get/create CRD client: %w", err)
	}

	isHypershift, err = operatorutil.CheckCRDExist(ctx, crdClient, "hostedclusters.hypershift.openshift.io")
	if err != nil {
		return false, fmt.Errorf("failed to check if the CRD hostedclusters.hypershift.openshift.io exists: %w", err)
	}

	return isHypershift, nil
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

	smDesired.ResourceVersion = smCurrent.ResourceVersion
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
	metricsList := []string{
		"etcd_disk_backend_commit_duration_seconds_bucket",
		"etcd_disk_wal_fsync_duration_seconds_bucket",
		"etcd_mvcc_db_total_size_in_bytes",
		"etcd_mvcc_db_total_size_in_use_in_bytes",
		"etcd_network_client_grpc_received_bytes_total",
		"etcd_network_client_grpc_sent_bytes_total",
		"etcd_network_peer_received_bytes_total",
		"etcd_network_peer_round_trip_time_seconds_bucket",
		"etcd_network_peer_sent_bytes_total",
		"etcd_server_has_leader",
		"etcd_server_leader_changes_seen_total",
		"etcd_server_leader_changes_seen_total",
		"etcd_server_proposals_applied_total",
		"etcd_server_proposals_committed_total",
		"etcd_server_proposals_failed_total",
		"etcd_server_proposals_pending",
		"grpc_server_handled_total",
		"grpc_server_started_total",
		"process_resident_memory_bytes",
	}

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
					BearerTokenSecret: &corev1.SecretKeySelector{},
					TLSConfig:         originalEndpoint.TLSConfig,
					MetricRelabelConfigs: []promv1.RelabelConfig{
						{
							SourceLabels: []promv1.LabelName{"__name__"},
							Action:       "keep",
							Regex:        fmt.Sprintf("(%s)", strings.Join(metricsList, "|")),
						},
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: &clusterID,
						},
						{
							TargetLabel: "cluster_id",
							Action:      "replace",
							Replacement: &clusterID,
						},
						{
							TargetLabel: "cluster",
							Action:      "replace",
							Replacement: &clusterName,
						},
					},
					RelabelConfigs: []promv1.RelabelConfig{
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: &clusterID,
						},
						{
							TargetLabel: "job",
							Action:      "replace",
							Replacement: stringPtr("etcd"),
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

	metricsList := []string{
		"apiserver_current_inflight_requests",
		"apiserver_request_count",
		"apiserver_request_duration_seconds_bucket",
		"apiserver_request_total",
		"apiserver_storage_objects",
		"go_goroutines",
		"process_cpu_seconds_total",
		"process_resident_memory_bytes",
		"up",
		"workqueue_adds_total",
		"workqueue_depth",
		"workqueue_queue_duration_seconds_bucket",
	}

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
					BearerTokenSecret: &corev1.SecretKeySelector{},
					TLSConfig:         originalEndpoint.TLSConfig,
					MetricRelabelConfigs: []promv1.RelabelConfig{
						{
							SourceLabels: []promv1.LabelName{"__name__"},
							Action:       "keep",
							Regex:        fmt.Sprintf("(%s)", strings.Join(metricsList, "|")),
						},
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: &clusterID,
						},
						{
							TargetLabel: "cluster_id",
							Action:      "replace",
							Replacement: &clusterID,
						},
						{
							TargetLabel: "cluster",
							Action:      "replace",
							Replacement: &clusterName,
						},
					},
					RelabelConfigs: []promv1.RelabelConfig{
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: &clusterID,
						},
						{
							TargetLabel: "job",
							Action:      "replace",
							Replacement: stringPtr("apiserver"),
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

func stringPtr(s string) *string {
	return &s
}
