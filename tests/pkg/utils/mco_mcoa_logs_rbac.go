// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const (
	mchPauseAnnotation            = "installer.open-cluster-management.io/pause"
	mcoMCHClusterRoleBindingName  = "open-cluster-management:multicluster-observability-operator:multicluster-observability-operator"
	mcoOperatorServiceAccountName = "multicluster-observability-operator"
)

var managedLogStoreRBACRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{"operators.coreos.com"},
		Resources: []string{"operatorgroups", "subscriptions"},
		Verbs:     []string{"create", "get", "list", "watch"},
	},
	{
		APIGroups: []string{"cert-manager.io"},
		Resources: []string{"issuers", "certificates", "clusterissuers"},
		Verbs:     []string{"create", "get", "list", "watch"},
	},
}

// ManagedLogStoreRBACOverride temporarily changes the ClusterRole that MCH owns.
// It is only used by the managed log store E2E context while ACM's installed
// bundle does not yet include the required RBAC rules.
type ManagedLogStoreRBACOverride struct {
	dynamicClient dynamic.Interface
	kubeClient    kubernetes.Interface
	mchName       string
	mchNamespace  string
	roleName      string

	originalPauseValue string
	hadPauseAnnotation bool
	originalRoleRules  []rbacv1.PolicyRule
	roleChanged        bool
}

func NewManagedLogStoreRBACOverride(ctx context.Context, opt TestOptions) (*ManagedLogStoreRBACOverride, error) {
	return newManagedLogStoreRBACOverride(ctx, GetKubeClientDynamic(opt, true), GetKubeClient(opt, true))
}

func newManagedLogStoreRBACOverride(
	ctx context.Context, dynamicClient dynamic.Interface, kubeClient kubernetes.Interface,
) (*ManagedLogStoreRBACOverride, error) {
	mchList, err := dynamicClient.Resource(NewOCMMultiClusterHubGVR()).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list MultiClusterHubs: %w", err)
	}
	if len(mchList.Items) != 1 {
		return nil, fmt.Errorf("expected exactly one MultiClusterHub, found %d", len(mchList.Items))
	}
	mch := &mchList.Items[0]
	pauseValue, hadPauseAnnotation := mch.GetAnnotations()[mchPauseAnnotation]
	if pauseValue == "true" {
		return nil, fmt.Errorf("MultiClusterHub %s/%s is already paused", mch.GetNamespace(), mch.GetName())
	}

	binding, err := kubeClient.RbacV1().ClusterRoleBindings().Get(ctx, mcoMCHClusterRoleBindingName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get MCH-managed MCO ClusterRoleBinding: %w", err)
	}
	if binding.RoleRef.Kind != "ClusterRole" {
		return nil, fmt.Errorf("MCO ClusterRoleBinding %s references %s instead of a ClusterRole", binding.Name, binding.RoleRef.Kind)
	}
	boundToMCO := false
	for _, subject := range binding.Subjects {
		if subject.Kind == rbacv1.ServiceAccountKind && subject.Name == mcoOperatorServiceAccountName && subject.Namespace == mch.GetNamespace() {
			boundToMCO = true
			break
		}
	}
	if !boundToMCO {
		return nil, fmt.Errorf("MCO ClusterRoleBinding %s does not bind the MCO operator service account in %s", binding.Name, mch.GetNamespace())
	}

	role, err := kubeClient.RbacV1().ClusterRoles().Get(ctx, binding.RoleRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get MCH-managed MCO ClusterRole %s: %w", binding.RoleRef.Name, err)
	}

	return &ManagedLogStoreRBACOverride{
		dynamicClient:      dynamicClient,
		kubeClient:         kubeClient,
		mchName:            mch.GetName(),
		mchNamespace:       mch.GetNamespace(),
		roleName:           role.Name,
		originalPauseValue: pauseValue,
		hadPauseAnnotation: hadPauseAnnotation,
		originalRoleRules:  append([]rbacv1.PolicyRule(nil), role.Rules...),
	}, nil
}

func (o *ManagedLogStoreRBACOverride) Pause(ctx context.Context) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		mch, err := o.dynamicClient.Resource(NewOCMMultiClusterHubGVR()).Namespace(o.mchNamespace).
			Get(ctx, o.mchName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		annotations := mch.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[mchPauseAnnotation] = "true"
		mch.SetAnnotations(annotations)
		_, err = o.dynamicClient.Resource(NewOCMMultiClusterHubGVR()).Namespace(o.mchNamespace).
			Update(ctx, mch, metav1.UpdateOptions{})
		return err
	})
}

func (o *ManagedLogStoreRBACOverride) CheckPaused(ctx context.Context, paused bool) error {
	mch, err := o.dynamicClient.Resource(NewOCMMultiClusterHubGVR()).Namespace(o.mchNamespace).
		Get(ctx, o.mchName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	phase, _, err := unstructured.NestedString(mch.Object, "status", "phase")
	if err != nil {
		return fmt.Errorf("read MultiClusterHub phase: %w", err)
	}
	if (phase == "Paused") != paused {
		return fmt.Errorf("MultiClusterHub %s/%s phase is %q, want paused=%t", o.mchNamespace, o.mchName, phase, paused)
	}
	return nil
}

func (o *ManagedLogStoreRBACOverride) ApplyRBAC(ctx context.Context) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		role, err := o.kubeClient.RbacV1().ClusterRoles().Get(ctx, o.roleName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		changed := false
		for _, needed := range managedLogStoreRBACRules {
			found := false
			for _, existing := range role.Rules {
				if reflect.DeepEqual(existing, needed) {
					found = true
					break
				}
			}
			if !found {
				role.Rules = append(role.Rules, needed)
				changed = true
			}
		}
		if !changed {
			return nil
		}
		// Restore the original rules even if the update response is lost after the API accepted it.
		o.roleChanged = true
		_, err = o.kubeClient.RbacV1().ClusterRoles().Update(ctx, role, metav1.UpdateOptions{})
		return err
	})
}

// Restore removes the temporary RBAC before resuming MCH reconciliation.
// Both restores are attempted so an RBAC error cannot leave MCH paused.
func (o *ManagedLogStoreRBACOverride) Restore(ctx context.Context) error {
	var roleErr error
	if o.roleChanged {
		roleErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			role, err := o.kubeClient.RbacV1().ClusterRoles().Get(ctx, o.roleName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			role.Rules = append([]rbacv1.PolicyRule(nil), o.originalRoleRules...)
			_, err = o.kubeClient.RbacV1().ClusterRoles().Update(ctx, role, metav1.UpdateOptions{})
			return err
		})
	}

	pauseErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		mch, err := o.dynamicClient.Resource(NewOCMMultiClusterHubGVR()).Namespace(o.mchNamespace).
			Get(ctx, o.mchName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		annotations := mch.GetAnnotations()
		if o.hadPauseAnnotation {
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[mchPauseAnnotation] = o.originalPauseValue
		} else {
			delete(annotations, mchPauseAnnotation)
		}
		mch.SetAnnotations(annotations)
		_, err = o.dynamicClient.Resource(NewOCMMultiClusterHubGVR()).Namespace(o.mchNamespace).
			Update(ctx, mch, metav1.UpdateOptions{})
		return err
	})
	return errors.Join(roleErr, pauseErr)
}
