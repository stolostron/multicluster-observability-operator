// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
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

func TestCreateRoleBinding(t *testing.T) {
	c := fake.NewFakeClient()
	err := createRoleBinding(c, namespace)
	if err != nil {
		t.Fatalf("createRole: (%v)", err)
	}
	found := &rbacv1.RoleBinding{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: roleBindingName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to create RoleBinding: (%v)", err)
	}
	if found.RoleRef.Name != roleName || found.Subjects[0].Name != serviceAccountName {
		t.Fatalf("rolebinding is no created correctly")
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: namespace,
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     roleName + "-test",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName + "-test",
				Namespace: namespace,
			},
		},
	}
	objs := []runtime.Object{rb}
	c = fake.NewFakeClient(objs...)
	err = createRoleBinding(c, namespace)
	if err != nil {
		t.Fatalf("createRoleBinding: (%v)", err)
	}
	found = &rbacv1.RoleBinding{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: roleBindingName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to update RoleBinding: (%v)", err)
	}
	if found.RoleRef.Name != roleName || found.Subjects[0].Name != serviceAccountName {
		t.Fatalf("rolebinding is no updated correctly")
	}
}

func TestCreateServiceAccount(t *testing.T) {
	c := fake.NewFakeClient()
	err := createServiceAccount(c, namespace)
	if err != nil {
		t.Fatalf("createServiceAccount: (%v)", err)
	}
	found := &corev1.ServiceAccount{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName, Namespace: namespace}, found)
	if err != nil {
		t.Fatalf("Failed to create ServiceAccount: (%v)", err)
	}
	if found.Name != serviceAccountName {
		t.Fatalf("serviceaccount is no created correctly")
	}
}

func TestGetSAToken(t *testing.T) {
	secretName := "test-secret"
	token := "test-token"
	ca := "test-ca"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			"token":  []byte(token),
			"ca.crt": []byte(ca),
		},
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Secrets: []corev1.ObjectReference{
			{
				Kind:      "Secret",
				Namespace: namespace,
				Name:      secretName,
			},
		},
	}
	objs := []runtime.Object{secret, sa}
	c := fake.NewFakeClient(objs...)
	saCA, saToken, err := getSAToken(c, namespace)
	if err != nil {
		t.Fatalf("Failed to get ServiceAccount Token: (%v)", err)
	}
	if string(saCA) != ca || string(saToken) != token {
		t.Fatal("Got wrong ca/token")
	}

}
