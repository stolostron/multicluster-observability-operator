// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package openshift

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	ocinfrav1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ClusterRoleBindingName    = "metrics-collector-view"
	HubClusterRoleBindingName = "hub-metrics-collector-view"
	CaConfigmapName           = "metrics-collector-serving-certs-ca-bundle"
	OwnerLabelKey             = "owner"
	OwnerLabelValue           = "observabilityaddon"
)

func DeleteMonitoringClusterRoleBinding(ctx context.Context, client client.Client, isHubMetricsCollector bool) error {
	clusterRoleBindingName := ClusterRoleBindingName
	if isHubMetricsCollector {
		clusterRoleBindingName = HubClusterRoleBindingName
	}
	rb := &rbacv1.ClusterRoleBinding{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      clusterRoleBindingName,
		Namespace: "",
	}, rb)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to check clusterrolebinding: %w", err)
	}
	err = client.Delete(ctx, rb)
	if err != nil {
		return fmt.Errorf("failed to delete clusterrolebinding: %w", err)
	}
	return nil
}

func CreateMonitoringClusterRoleBinding(ctx context.Context, log logr.Logger, client client.Client, namespace, serviceAccountName string, isHubMetricsCollector bool) error {
	clusterRoleBindingName := ClusterRoleBindingName
	if isHubMetricsCollector {
		clusterRoleBindingName = HubClusterRoleBindingName
	}
	saSubject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      serviceAccountName,
		Namespace: namespace,
	}

	rb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleBindingName,
			Annotations: map[string]string{
				OwnerLabelKey: OwnerLabelValue,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "cluster-monitoring-view",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{saSubject},
	}

	found := &rbacv1.ClusterRoleBinding{}
	err := client.Get(ctx, types.NamespacedName{Name: clusterRoleBindingName, Namespace: ""}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := client.Create(ctx, rb); err != nil {
				return fmt.Errorf("failed to create clusterrolebinding: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to check clusterrolebinding: %w", err)
	}

	if reflect.DeepEqual(rb.RoleRef, found.RoleRef) && reflect.DeepEqual(rb.Subjects, found.Subjects) {
		return nil
	}
	rb.ResourceVersion = found.ResourceVersion
	err = client.Update(ctx, rb)
	if err != nil {
		log.Error(err, "Failed to update the clusterrolebinding")
	}

	return nil
}

func DeleteCAConfigmap(ctx context.Context, client client.Client, namespace string) error {
	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      CaConfigmapName,
		Namespace: namespace,
	}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to check configmap: %w", err)
	}
	err = client.Delete(ctx, cm)
	if err != nil {
		return fmt.Errorf("failed to delete configmap: %w", err)
	}

	return nil
}

func CreateCAConfigmap(ctx context.Context, client client.Client, namespace string) error {
	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      CaConfigmapName,
		Namespace: namespace,
	}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      CaConfigmapName,
					Namespace: namespace,
					Annotations: map[string]string{
						OwnerLabelKey: OwnerLabelValue,
						"service.alpha.openshift.io/inject-cabundle": "true",
					},
				},
				Data: map[string]string{"service-ca.crt": ""},
			}
			if err := client.Create(ctx, cm); err != nil {
				return fmt.Errorf("failed to create configmap: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to check configmap: %w", err)
	}
	return nil
}

func IsSNO(ctx context.Context, c client.Client) (bool, error) {
	infraConfig := &ocinfrav1.Infrastructure{}
	if err := c.Get(ctx, types.NamespacedName{Name: "cluster"}, infraConfig); err != nil {
		parentErr := err
		isSNO, err := isSingleNode(ctx, c)
		if err != nil {
			return isSNO, fmt.Errorf("failed to get OCP infrastructure: %s; failed to check if single node: %w", parentErr, err)
		}

		return isSNO, nil
	}
	if infraConfig.Status.ControlPlaneTopology == ocinfrav1.SingleReplicaTopologyMode {
		return true, nil
	}

	return false, nil
}

func isSingleNode(ctx context.Context, c client.Client) (bool, error) {
	nodes := &corev1.NodeList{}
	opts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"node-role.kubernetes.io/master": ""}),
	}
	err := c.List(ctx, nodes, opts)
	if err != nil {
		return false, fmt.Errorf("failed to get node list: %w", err)
	}
	if len(nodes.Items) == 1 {
		return true, nil
	}
	return false, nil
}
