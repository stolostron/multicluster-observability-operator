// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	resRoleName        = "endpoint-observability-res-role"
	resRoleBindingName = "endpoint-observability-res-rolebinding"
	mcoRoleName        = "endpoint-observability-mco-role"
	mcoRoleBindingName = "endpoint-observability-mco-rolebinding"
	serviceAccountName = "endpoint-observability-sa"
	epRsName           = "observabilityaddons"
	epStatusRsName     = "observabilityaddons/status"
	mcoRsName          = "multiclusterobservabilities"
	epRsGroup          = "observability.open-cluster-management.io"
)

func createClusterRole(c client.Client) error {

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
	err := c.Get(context.TODO(), types.NamespacedName{Name: mcoRoleName, Namespace: ""}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating mco clusterRole")
		err = c.Create(context.TODO(), role)
		if err != nil {
			log.Error(err, "Failed to create endpoint-observability-mco-role clusterRole")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check endpoint-observability-mco-role clusterRole")
		return err
	}

	if !reflect.DeepEqual(found.Rules, role.Rules) {
		log.Info("Updating endpoint-observability-mco-role clusterRole")
		role.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = c.Update(context.TODO(), role)
		if err != nil {
			log.Error(err, "Failed to update endpoint-observability-mco-role clusterRole")
			return err
		}
		return nil
	}

	log.Info("clusterrole endpoint-observability-mco-role already existed/unchanged")
	return nil
}

func createClusterRoleBinding(c client.Client, namespace string) error {
	rb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace + "-" + mcoRoleBindingName,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     mcoRoleName,
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
	err := c.Get(context.TODO(), types.NamespacedName{Name: namespace + "-" +
		mcoRoleBindingName, Namespace: ""}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating endpoint-observability-mco-rolebinding clusterrolebinding")
		err = c.Create(context.TODO(), rb)
		if err != nil {
			log.Error(err, "Failed to create endpoint-observability-mco-rolebinding clusterrolebinding")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check endpoint-observability-mco-rolebinding clusterrolebinding")
		return err
	}

	if !reflect.DeepEqual(found.Subjects, rb.Subjects) && !reflect.DeepEqual(found.RoleRef, rb.RoleRef) {
		log.Info("Updating endpoint-observability-mco-rolebinding clusterrolebinding")
		rb.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = c.Update(context.TODO(), rb)
		if err != nil {
			log.Error(err, "Failed to update endpoint-observability-mco-rolebinding clusterrolebinding")
			return err
		}
		return nil
	}

	log.Info("clusterrolebinding endpoint-observability-mco-rolebinding already existed/unchanged", "namespace", namespace)
	return nil
}

func createResourceRole(c client.Client) error {

	deleteDeprecatedRoles(c)
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: resRoleName,
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
			{
				Resources: []string{
					"leases",
				},
				Verbs: []string{
					"watch",
					"list",
					"get",
					"create",
					"update",
				},
				APIGroups: []string{
					"coordination.k8s.io",
				},
			},
			{
				Resources: []string{
					"managedclusteraddons",
					"managedclusteraddons/status",
				},
				Verbs: []string{
					"watch",
					"list",
					"get",
					"update",
				},
				APIGroups: []string{
					"addon.open-cluster-management.io",
				},
			},
		},
	}

	found := &rbacv1.ClusterRole{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: resRoleName, Namespace: ""}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating endpoint-observability-res-role clusterrole")
		err = c.Create(context.TODO(), role)
		if err != nil {
			log.Error(err, "Failed to create endpoint-observability-res-role clusterrole")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check endpoint-observability-res-role clusterrole")
		return err
	}

	if !reflect.DeepEqual(found.Rules, role.Rules) {
		log.Info("Updating endpoint-observability-res-role clusterrole")
		role.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = c.Update(context.TODO(), role)
		if err != nil {
			log.Error(err, "Failed to update endpoint-observability-res-role clusterrole")
			return err
		}
		return nil
	}

	log.Info("clusterrole endpoint-observability-res-role already existed/unchanged")
	return nil
}

func createResourceRoleBinding(c client.Client, namespace string) error {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resRoleBindingName,
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     resRoleName,
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
	err := c.Get(context.TODO(), types.NamespacedName{Name: resRoleBindingName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating endpoint-observability-res-rolebinding rolebinding", "namespace", namespace)
		err = c.Create(context.TODO(), rb)
		if err != nil {
			log.Error(err, "Failed to create endpoint-observability-res-rolebinding rolebinding", "namespace", namespace)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check endpoint-observability-res-rolebinding rolebinding", "namespace", namespace)
		return err
	}

	if !reflect.DeepEqual(found.Subjects, rb.Subjects) && !reflect.DeepEqual(found.RoleRef, rb.RoleRef) {
		log.Info("Updating endpoint-observability-res-rolebinding rolebinding", "namespace", namespace)
		rb.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = c.Update(context.TODO(), rb)
		if err != nil {
			log.Error(err, "Failed to update endpoint-observability-res-rolebinding rolebinding", "namespace", namespace)
			return err
		}
		return nil
	}

	log.Info("rolebinding endpoint-observability-res-rolebinding already existed/unchanged", "namespace", namespace)
	return nil
}

func createServiceAccount(c client.Client, namespace string) error {
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
	err := c.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating endpoint-observability-controller-sa serviceaccount", "namespace", namespace)
		err = c.Create(context.TODO(), sa)
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

func getSAToken(c client.Client, namespace string) ([]byte, []byte, error) {

	err := createClusterRoleBinding(c, namespace)
	if err != nil {
		return nil, nil, err
	}
	err = createResourceRoleBinding(c, namespace)
	if err != nil {
		return nil, nil, err
	}
	err = createServiceAccount(c, namespace)
	if err != nil {
		return nil, nil, err
	}
	saFound := &corev1.ServiceAccount{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName, Namespace: namespace}, saFound)
	if err != nil {
		log.Error(err, "Failed to get endpoint-observability-controller-sa serviceaccount", "namespace", namespace)
		return nil, nil, err
	}
	secrets := saFound.Secrets
	for _, s := range secrets {
		secretFound := &corev1.Secret{}
		err = c.Get(context.TODO(), types.NamespacedName{Name: s.Name, Namespace: namespace}, secretFound)
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

func deleteClusterRole(c client.Client) error {
	clusterrole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoRoleName,
		},
	}
	err := c.Delete(context.TODO(), clusterrole)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete clusterrole", "name", mcoRoleName)
		return err
	}
	log.Info("Clusterrole deleted", "name", mcoRoleName)
	return nil
}

func deleteResourceRole(c client.Client) error {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: resRoleName,
		},
	}
	err := c.Delete(context.TODO(), role)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete clusterrole", "name", resRoleName)
		return err
	}
	log.Info("Role deleted", "name", resRoleName)
	return nil
}

func deleteRes(c client.Client, namespace string) error {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace + "-" + resRoleBindingName,
		},
	}
	err := c.Delete(context.TODO(), crb)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete clusterrolebinding", "name", namespace+"-"+resRoleBindingName)
		return err
	}
	log.Info("Clusterrolebinding deleted", "name", namespace+"-"+resRoleBindingName)

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resRoleBindingName,
			Namespace: namespace,
		},
	}
	err = c.Delete(context.TODO(), rb)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete rolebinding", "name", resRoleBindingName, "namespace", namespace)
		return err
	}
	log.Info("Rolebinding deleted", "name", resRoleBindingName, "namespace", namespace)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
		},
	}
	err = c.Delete(context.TODO(), sa)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete serviceaccount", "name", serviceAccountName, "namespace", namespace)
		return err
	}
	log.Info("Serviceaccount deleted", "name", serviceAccountName, "namespace", namespace)

	return nil
}

// function to remove the deprecated roles
func deleteDeprecatedRoles(c client.Client) {
	opts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{ownerLabelKey: ownerLabelValue}),
	}
	roleList := &rbacv1.RoleList{}
	err := c.List(context.TODO(), roleList, opts)
	if err != nil {
		log.Error(err, "Failed to list deprecated roles")
		return
	}
	for _, role := range roleList.Items {
		if role.Name == "endpoint-observability-role" {
			err = c.Delete(context.TODO(), &role)
			if err != nil && !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete deprecated roles", "name", role.Name, "namespace", role.Namespace)
			} else {
				log.Info("Deprecated role deleted", "name", role.Name, "namespace", role.Namespace)
			}
		}
	}
}
