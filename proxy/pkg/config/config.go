// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package config

import (
	"context"
	"fmt"

	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

const (
	ManagedClusterLabelAllowListConfigMapName = "observability-managed-cluster-label-allowlist"
	ManagedClusterLabelAllowListConfigMapKey  = "managed_cluster.yaml"
	ManagedClusterLabelAllowListNamespace     = "open-cluster-management-observability"

	RBACProxyLabelMetricName              = "acm_label_names"
	ACMManagedClusterLabelNamesMetricName = "acm_managed_cluster_labels"
)

var (
	RequiredLabelList = []string{"name", "cluster.open-cluster-management.io/clusterset"}
	// Scheme is the runtime scheme for the proxy.
	Scheme = runtime.NewScheme()
)

func init() {
	_ = userv1.AddToScheme(Scheme)
	_ = projectv1.AddToScheme(Scheme)
}

// CreateManagedClusterLabelAllowListCM creates a managedcluster label allowlist configmap object.
func CreateManagedClusterLabelAllowListCM(namespace string) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ManagedClusterLabelAllowListConfigMapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			ManagedClusterLabelAllowListConfigMapKey: `labels:
- cloud
- vendor

ignore_labels:
- clusterID
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
`}}
}

// GetManagedClusterLabelAllowListConfigmap returns the managedcluster label allowlist configmap.
func GetManagedClusterLabelAllowListConfigmap(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (*v1.ConfigMap,
	error) {
	configmap, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(
		ctx,
		ManagedClusterLabelAllowListConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get managedcluster label allowlist configmap: %w", err)
	}
	return configmap, nil
}
