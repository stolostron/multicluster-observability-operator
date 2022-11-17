// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"

	"gopkg.in/yaml.v2"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"k8s.io/klog"
	"k8s.io/kubectl/pkg/util/slice"
)

const (
	ManagedClusterLabelAllowListConfigMapName = "observability-managed-cluster-label-allowlist"
	ManagedClusterLabelAllowListConfigMapKey  = "managed_cluster.yaml"
	ManagedClusterLabelMetricName             = "managed_cluster_labels"
	ManagedClusterLabelAllowListNamespace     = "open-cluster-management-observability"

	RBACProxyLabelMetricName = "acm_label_names"
)

var (
	ManagedLabelList ManagedClusterLabelList
	SyncLabelList    ManagedClusterLabelList
)

// GetManagedClusterLabelAllowListConfigMapKey return the key name for the managedcluster labels
func GetManagedClusterLabelAllowListConfigMapKey() string {
	return ManagedClusterLabelAllowListConfigMapKey
}

// GetManagedClusterLabelConfigMapName return the name for the managedcluster labels configmap
func GetManagedClusterLabelAllowListConfigMapName() string {
	return ManagedClusterLabelAllowListConfigMapName
}

// GetManagedClusterLabelMetricName return the name of the managedcluster label metric
func GetManagedClusterLabelMetricName() string {
	return ManagedClusterLabelMetricName
}

// GetManagedClusterLabelList will return the current cluster label list
func GetManagedClusterLabelList() *ManagedClusterLabelList {
	return &ManagedLabelList
}

// GetSyncLabelList will return the synced label list
func GetSyncLabelList() *ManagedClusterLabelList {
	return &SyncLabelList
}

// GetRBACProxyLabelMetricName returns the name of the rbac query proxy label metric
func GetRBACProxyLabelMetricName() string {
	return RBACProxyLabelMetricName
}

// CreateManagedClusterLabelAllowListCM creates a managedcluster label allowlist configmap object
func CreateManagedClusterLabelAllowListCM(namespace string) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetManagedClusterLabelAllowListConfigMapName(),
			Namespace: namespace,
		},
		Data: map[string]string{
			GetManagedClusterLabelAllowListConfigMapKey(): `labels:
- cloud
- vendor

ignore_labels:
- clusterID
- cluster.open-cluster-management.io/clusterset
- feature.open-cluster-management.io/addon-application-manager
- feature.open-cluster-management.io/addon-cert-policy-controller
- feature.open-cluster-management.io/addon-cluster-proxy
- feature.open-cluster-management.io/addon-config-policy-controller
- feature.open-cluster-management.io/addon-governance-policy-framework
- feature.open-cluster-management.io/addon-iam-policy-controller
- feature.open-cluster-management.io/addon-observability-controller
- feature.open-cluster-management.io/addon-search-collector
- feature.open-cluster-management.io/addon-work-manager
- installer.name
- installer.namespace
- local-cluster
- name
`}}
}

// GetManagedClusterLabelAllowListConfigmap returns the managedcluster label allowlist configmap
func GetManagedClusterLabelAllowListConfigmap(CoreV1Interface corev1.CoreV1Interface, namespace string) (*v1.ConfigMap,
	error) {
	configmap, err := CoreV1Interface.ConfigMaps(namespace).Get(
		context.TODO(),
		GetManagedClusterLabelAllowListConfigMapName(),
		metav1.GetOptions{},
	)
	if err != nil {
		klog.Errorf("failed to get managedcluster label allowlist configmap: %v", err)
		return nil, err
	}
	return configmap, nil
}

// ModifyManagedClusterLabelAllowListConfigMapData modifies the data for managedcluster label allowlist
func ModifyManagedClusterLabelAllowListConfigMapData(cm *v1.ConfigMap, clusterLabels map[string]string) error {
	var clusterLabelList = &ManagedClusterLabelList{}

	err := yaml.Unmarshal(
		[]byte(cm.Data[GetManagedClusterLabelAllowListConfigMapKey()]),
		clusterLabelList,
	)
	if err != nil {
		klog.Errorf("failed to unmarshal configmap <%s> data to the clusterLabelList: %v",
			GetManagedClusterLabelAllowListConfigMapKey(), err)
		return err
	}

	for key := range clusterLabels {
		if !slice.ContainsString(clusterLabelList.IgnoreList, key, nil) &&
			!slice.ContainsString(clusterLabelList.LabelList, key, nil) {
			clusterLabelList.LabelList = append(clusterLabelList.LabelList, key)
			klog.Infof("added label <%s> to managedcluster label allowlist", key)
		}
	}

	data, err := yaml.Marshal(clusterLabelList)
	if err != nil {
		klog.Errorf("failed to marshal data to clusterLabelList: %v", err)
		return err
	}
	cm.Data = map[string]string{GetManagedClusterLabelAllowListConfigMapKey(): string(data)}
	return nil
}

// UpdateManagedClusterLabelAllowListConfigMap updates the managedcluster label allowlist configmap
func UpdateManagedClusterLabelAllowListConfigMap(
	CoreV1Interface corev1.CoreV1Interface,
	cm *v1.ConfigMap,
) error {
	_, err := CoreV1Interface.ConfigMaps(cm.Namespace).Update(
		context.TODO(),
		cm,
		metav1.UpdateOptions{},
	)
	if err != nil {
		klog.Errorf("failed to update managedcluster label allowlist: %v", err)
		return err
	}
	return nil
}
