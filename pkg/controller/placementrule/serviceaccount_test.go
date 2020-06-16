// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	namespace = "test-ns"
)

func TestCreateRole(t *testing.T) {
	c := fake.NewFakeClient()
	err := createRole(c, namespace)
	if err != nil {
		t.Fatalf("createRole: (%v)", err)
	}
	found := &rbacv1.Role{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: roleName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to create Role: (%v)", err)
	}
	if len(found.Rules) != 2 {
		t.Fatalf("role is no created correctly")
	}

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
			Annotations: map[string]string{
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
					epRsGroup,
				},
			},
		},
	}
	objs := []runtime.Object{role}
	c = fake.NewFakeClient(objs...)
	err = createRole(c, namespace)
	if err != nil {
		t.Fatalf("createRole: (%v)", err)
	}
	found = &rbacv1.Role{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: roleName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to update Role: (%v)", err)
	}
	if len(found.Rules) != 2 {
		t.Fatalf("role is no updated correctly")
	}
}
