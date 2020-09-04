// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	roleName               = "endpoint-observability-role"
	roleBindingName        = "endpoint-observability-rolebinding"
	clusterRoleName        = "endpoint-observability-role"
	clusterRoleBindingName = "endpoint-observability-rolebinding"
	serviceAccountName     = "endpoint-observability-sa"
	epRsName               = "observabilityaddons"
	epStatusRsName         = "observabilityaddons/status"
	mcoRsName              = "multiclusterobservabilities"
	epRsGroup              = "observability.open-cluster-management.io"
)

func createClusterRole(client client.Client) error {

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				Resources: []string{
					mcoRsName,
				},
				Verbs: []string{
					"watch",
					"list",
					"get",
				},
				APIGroups: []string{
					epRsGroup,
				},
			},
		},
	}

	found := &rbacv1.ClusterRole{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: clusterRoleName, Namespace: ""}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating endpoint-observability-controller clusterRole")
		err = client.Create(context.TODO(), role)
		if err != nil {
			log.Error(err, "Failed to create endpoint-observability-controller clusterRole")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check endpoint-observability-controller clusterRole")
		return err
	}

	if !reflect.DeepEqual(found.Rules, role.Rules) {
		log.Info("Updating endpoint-observability-controller clusterRole")
		role.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), role)
		if err != nil {
			log.Error(err, "Failed to update endpoint-observability-controller clusterRole")
			return err
		}
		return nil
	}

	log.Info("clusterrole already existed/unchanged")
	return nil
}

func createClusterRoleBinding(client client.Client, namespace string) error {
	rb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace + "-" + roleBindingName,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		},
	}
	found := &rbacv1.ClusterRoleBinding{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: namespace + "-" + roleBindingName, Namespace: ""}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating endpoint-observability-controller clusterrolebinding")
		err = client.Create(context.TODO(), rb)
		if err != nil {
			log.Error(err, "Failed to create endpoint-observability-controller clusterrolebinding")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check endpoint-observability-controller clusterrolebinding")
		return err
	}

	if !reflect.DeepEqual(found.Subjects, rb.Subjects) && !reflect.DeepEqual(found.RoleRef, rb.RoleRef) {
		log.Info("Updating endpoint-observability-controller clusterrolebinding")
		rb.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), rb)
		if err != nil {
			log.Error(err, "Failed to update endpoint-observability-controller clusterrolebinding")
			return err
		}
		return nil
	}

	log.Info("clusterrolebinding already existed/unchanged", "namespace", namespace)
	return nil
}

func createRole(client client.Client, namespace string) error {

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				Resources: []string{
					epRsName,
					epStatusRsName,
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
			{
				Resources: []string{
					"pods",
					"secrets",
				},
				Verbs: []string{
					"watch",
					"list",
					"get",
				},
				APIGroups: []string{
					"",
				},
			},
		},
	}

	found := &rbacv1.Role{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: roleName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating endpoint-observability-controller role", "namespace", namespace)
		err = client.Create(context.TODO(), role)
		if err != nil {
			log.Error(err, "Failed to create endpoint-observability-controller role")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check endpoint-observability-controller role")
		return err
	}

	if !reflect.DeepEqual(found.Rules, role.Rules) {
		log.Info("Updating endpoint-observability-controller role", "namespace", namespace)
		role.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), role)
		if err != nil {
			log.Error(err, "Failed to update endpoint-observability-controller role")
			return err
		}
		return nil
	}

	log.Info("role already existed/unchanged", "namespace", namespace)
	return nil
}

func createRoleBinding(client client.Client, namespace string) error {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     roleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		},
	}
	found := &rbacv1.RoleBinding{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: roleBindingName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating endpoint-observability-controller rolebinding", "namespace", namespace)
		err = client.Create(context.TODO(), rb)
		if err != nil {
			log.Error(err, "Failed to create endpoint-observability-controller rolebinding")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check endpoint-observability-controller rolebinding")
		return err
	}

	if !reflect.DeepEqual(found.Subjects, rb.Subjects) && !reflect.DeepEqual(found.RoleRef, rb.RoleRef) {
		log.Info("Updating endpoint-observability-controller rolebinding", "namespace", namespace)
		rb.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), rb)
		if err != nil {
			log.Error(err, "Failed to update endpoint-observability-controller rolebinding")
			return err
		}
		return nil
	}

	log.Info("rolebinding already existed/unchanged", "namespace", namespace)
	return nil
}

func createServiceAccount(client client.Client, namespace string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
	}
	found := &corev1.ServiceAccount{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating endpoint-observability-controller-sa serviceaccount", "namespace", namespace)
		err = client.Create(context.TODO(), sa)
		if err != nil {
			log.Error(err, "Failed to create endpoint-observability-controller-sa serviceaccount")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check endpoint-observability-controller-sa serviceaccount")
		return err
	}

	log.Info("serviceaccount already existed/unchanged", "namespace", namespace)
	return nil
}

func getSAToken(client client.Client, namespace string) ([]byte, []byte, error) {

	err := createClusterRoleBinding(client, namespace)
	if err != nil {
		return nil, nil, err
	}
	err = createRole(client, namespace)
	if err != nil {
		return nil, nil, err
	}
	err = createRoleBinding(client, namespace)
	if err != nil {
		return nil, nil, err
	}
	err = createServiceAccount(client, namespace)
	if err != nil {
		return nil, nil, err
	}
	saFound := &corev1.ServiceAccount{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName, Namespace: namespace}, saFound)
	if err != nil {
		log.Error(err, "Failed to get endpoint-observability-controller-sa serviceaccount", "namespace", namespace)
		return nil, nil, err
	}
	secrets := saFound.Secrets
	for _, s := range secrets {
		secretFound := &corev1.Secret{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: s.Name, Namespace: namespace}, secretFound)
		if err != nil {
			log.Error(err, "Failed to get secret", "secret", s.Name)
			return nil, nil, err
		}
		if secretFound.Type == corev1.SecretTypeServiceAccountToken {
			token := secretFound.Data["token"]
			ca := secretFound.Data["ca.crt"]
			return ca, token, nil
		}
	}
	return nil, nil, errors.NewNotFound(corev1.Resource("secret"), saFound.Name+"-token-*")
}

func deleteClusterRole(client client.Client) error {
	clusterrole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName,
		},
	}
	err := client.Delete(context.TODO(), clusterrole)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete clusterrole", "name", clusterRoleName)
		return err
	}
	log.Info("Clusterrole deleted", "name", clusterRoleName)
	return nil
}

func deleteRes(client client.Client, namespace string) error {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace + "-" + roleBindingName,
		},
	}
	err := client.Delete(context.TODO(), crb)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete clusterrolebinding", "name", namespace+"-"+roleBindingName)
		return err
	}
	log.Info("Clusterrolebinding deleted", "name", namespace+"-"+roleBindingName)

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
		},
	}
	err = client.Delete(context.TODO(), role)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete role", "name", roleName, "namespace", namespace)
		return err
	}
	log.Info("Role deleted", "name", roleName, "namespace", namespace)

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: namespace,
		},
	}
	err = client.Delete(context.TODO(), rb)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete rolebinding", "name", roleBindingName, "namespace", namespace)
		return err
	}
	log.Info("Rolebinding deleted", "name", roleBindingName, "namespace", namespace)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
		},
	}
	err = client.Delete(context.TODO(), sa)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete serviceaccount", "name", serviceAccountName, "namespace", namespace)
		return err
	}
	log.Info("Serviceaccount deleted", "name", serviceAccountName, "namespace", namespace)

	return nil
}
