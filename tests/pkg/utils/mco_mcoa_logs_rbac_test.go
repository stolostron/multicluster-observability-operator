// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"reflect"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestManagedLogStoreRBACOverrideRestoresClusterState(t *testing.T) {
	ctx := context.Background()
	mch := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "operator.open-cluster-management.io/v1",
		"kind":       "MultiClusterHub",
		"metadata": map[string]any{
			"name":      "multiclusterhub",
			"namespace": "open-cluster-management",
		},
		"status": map[string]any{"phase": "Running"},
	}}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{NewOCMMultiClusterHubGVR(): "MultiClusterHubList"},
		mch,
	)
	originalRule := rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}
	kubeClient := kubefake.NewSimpleClientset(
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: mcoMCHClusterRoleBindingName},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "mco-role"},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: mcoOperatorServiceAccountName, Namespace: "open-cluster-management"}},
		},
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "mco-role"},
			Rules:      []rbacv1.PolicyRule{originalRule},
		},
	)

	override, err := newManagedLogStoreRBACOverride(ctx, dynamicClient, kubeClient)
	if err != nil {
		t.Fatalf("create override: %v", err)
	}
	if err := override.Pause(ctx); err != nil {
		t.Fatalf("pause MCH: %v", err)
	}
	pausedMCH, err := dynamicClient.Resource(NewOCMMultiClusterHubGVR()).Namespace("open-cluster-management").Get(ctx, "multiclusterhub", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get paused MCH: %v", err)
	}
	if got := pausedMCH.GetAnnotations()[mchPauseAnnotation]; got != "true" {
		t.Fatalf("pause annotation = %q, want true", got)
	}
	if err := override.ApplyRBAC(ctx); err != nil {
		t.Fatalf("apply RBAC: %v", err)
	}
	if err := override.ApplyRBAC(ctx); err != nil {
		t.Fatalf("apply RBAC again: %v", err)
	}
	role, err := kubeClient.RbacV1().ClusterRoles().Get(ctx, "mco-role", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get updated ClusterRole: %v", err)
	}
	if want := 1 + len(managedLogStoreRBACRules); len(role.Rules) != want {
		t.Fatalf("ClusterRole has %d rules, want %d after repeated apply", len(role.Rules), want)
	}
	for _, needed := range managedLogStoreRBACRules {
		found := false
		for _, rule := range role.Rules {
			if reflect.DeepEqual(rule, needed) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ClusterRole does not contain rule %+v", needed)
		}
	}

	if err := override.Restore(ctx); err != nil {
		t.Fatalf("restore cluster state: %v", err)
	}
	role, err = kubeClient.RbacV1().ClusterRoles().Get(ctx, "mco-role", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get restored ClusterRole: %v", err)
	}
	if !reflect.DeepEqual(role.Rules, []rbacv1.PolicyRule{originalRule}) {
		t.Errorf("restored ClusterRole rules = %+v, want original rule", role.Rules)
	}
	restoredMCH, err := dynamicClient.Resource(NewOCMMultiClusterHubGVR()).Namespace("open-cluster-management").Get(ctx, "multiclusterhub", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get restored MCH: %v", err)
	}
	if _, present := restoredMCH.GetAnnotations()[mchPauseAnnotation]; present {
		t.Error("MCH pause annotation remained after restore")
	}
}

func TestManagedLogStoreRBACOverrideRejectsAlreadyPausedHub(t *testing.T) {
	ctx := context.Background()
	mch := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "operator.open-cluster-management.io/v1",
		"kind":       "MultiClusterHub",
		"metadata": map[string]any{
			"name":      "multiclusterhub",
			"namespace": "open-cluster-management",
			"annotations": map[string]any{
				mchPauseAnnotation: "true",
			},
		},
	}}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{NewOCMMultiClusterHubGVR(): "MultiClusterHubList"},
		mch,
	)
	if _, err := newManagedLogStoreRBACOverride(ctx, dynamicClient, kubefake.NewSimpleClientset()); err == nil {
		t.Fatal("expected an error when MCH is already paused")
	}
}
