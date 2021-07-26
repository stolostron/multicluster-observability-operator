// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"fmt"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

const (
	secretName = "test-secret"
	token      = "test-token"
	ca         = "test-ca"
)

func TestCreateClusterRole(t *testing.T) {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoRoleName,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				Resources: []string{
					config.MCORsName,
				},
				Verbs: []string{
					"watch",
					"list",
					"get",
				},
				APIGroups: []string{
					mcov1beta2.GroupVersion.Group,
				},
			},
			{
				Resources: []string{
					"pods",
				},
				Verbs: []string{
					"watch",
				},
				APIGroups: []string{
					"",
				},
			},
		},
	}
	objs := []runtime.Object{role}
	c := fake.NewFakeClient(objs...)
	err := createClusterRole(c)
	if err != nil {
		t.Fatalf("createRole: (%v)", err)
	}
	found := &rbacv1.ClusterRole{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: mcoRoleName}, found)
	if err != nil {
		t.Fatalf("Failed to update mcoClusterRole: (%v)", err)
	}
	if len(found.Rules) != 1 {
		t.Fatalf("role is no updated correctly")
	}
}

func TestCreateClusterRoleBinding(t *testing.T) {
	rb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace + "-" + mcoRoleBindingName,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     mcoRoleName + "-test",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "Group",
				Name:      "test",
				Namespace: namespace,
			},
		},
	}
	objs := []runtime.Object{rb}
	c := fake.NewFakeClient(objs...)
	err := createClusterRoleBinding(c, namespace, namespace)
	if err != nil {
		t.Fatalf("createRoleBinding: (%v)", err)
	}
	found := &rbacv1.ClusterRoleBinding{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: namespace + "-" + mcoRoleBindingName}, found)
	if err != nil {
		t.Fatalf("Failed to update ClusterRoleBinding: (%v)", err)
	}
	if found.RoleRef.Name != mcoRoleName ||
		found.Subjects[0].Name != fmt.Sprintf("system:open-cluster-management:cluster:%s:addon:%s", namespace, addonName) {
		t.Fatalf("clusterrolebinding is no updated correctly")
	}
}

func TestCreateRole(t *testing.T) {
	c := fake.NewFakeClient()
	err := createResourceRole(c)
	if err != nil {
		t.Fatalf("createRole: (%v)", err)
	}
	found := &rbacv1.ClusterRole{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: resRoleName, Namespace: ""}, found)
	if err != nil {
		t.Fatalf("Failed to create Role: (%v)", err)
	}
	if len(found.Rules) != 4 {
		t.Fatalf("role is no created correctly")
	}

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resRoleName,
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				Resources: []string{
					epRsName,
				},
				Verbs: []string{
					"watch",
					"list",
					"get",
					"update",
				},
				APIGroups: []string{
					mcov1beta2.GroupVersion.Group,
				},
			},
		},
	}
	objs := []runtime.Object{role}
	c = fake.NewFakeClient(objs...)
	err = createResourceRole(c)
	if err != nil {
		t.Fatalf("createRole: (%v)", err)
	}
	found = &rbacv1.ClusterRole{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: resRoleName, Namespace: ""}, found)
	if err != nil {
		t.Fatalf("Failed to update Role: (%v)", err)
	}
	if len(found.Rules) != 4 {
		t.Fatalf("role is no updated correctly")
	}
}

func TestCreateRoleBinding(t *testing.T) {
	c := fake.NewFakeClient()
	err := createResourceRoleBinding(c, namespace, namespace)
	if err != nil {
		t.Fatalf("createRole: (%v)", err)
	}
	found := &rbacv1.RoleBinding{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: resRoleBindingName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to create RoleBinding: (%v)", err)
	}
	if found.RoleRef.Name != resRoleName ||
		found.Subjects[0].Name != fmt.Sprintf("system:open-cluster-management:cluster:%s:addon:%s", namespace, addonName) {
		t.Fatalf("rolebinding is no created correctly")
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resRoleBindingName,
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     resRoleName + "-test",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "Group",
				Name:      "test",
				Namespace: namespace,
			},
		},
	}
	objs := []runtime.Object{rb}
	c = fake.NewFakeClient(objs...)
	err = createResourceRoleBinding(c, namespace, namespace)
	if err != nil {
		t.Fatalf("createRoleBinding: (%v)", err)
	}
	found = &rbacv1.RoleBinding{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: resRoleBindingName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to update RoleBinding: (%v)", err)
	}
	if found.RoleRef.Name != resRoleName ||
		found.Subjects[0].Name != fmt.Sprintf("system:open-cluster-management:cluster:%s:addon:%s", namespace, addonName) {
		t.Fatalf("rolebinding is no updated correctly")
	}
}
