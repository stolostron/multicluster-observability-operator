// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"fmt"
	"reflect"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

const (
	addonName          = "observability-controller"
	resRoleName        = "endpoint-observability-res-role"
	resRoleBindingName = "endpoint-observability-res-rolebinding"
	mcoRoleName        = "endpoint-observability-mco-role"
	mcoRoleBindingName = "endpoint-observability-mco-rolebinding"
	epRsName           = "observabilityaddons"
	epStatusRsName     = "observabilityaddons/status"
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
		},
	}

	found := &rbacv1.ClusterRole{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: mcoRoleName}, found)
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

func createClusterRoleBinding(c client.Client, namespace string, name string) error {
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
				Kind:      "Group",
				Name:      fmt.Sprintf("system:open-cluster-management:cluster:%s:addon:%s", name, addonName),
				Namespace: namespace,
			},
		},
	}
	found := &rbacv1.ClusterRoleBinding{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: namespace + "-" +
		mcoRoleBindingName}, found)
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
					mcov1beta2.GroupVersion.Group,
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
			{
				Resources: []string{
					"leases",
				},
				Verbs: []string{
					"watch",
					"list",
					"get",
					"update",
					"create",
					"delete",
				},
				APIGroups: []string{
					"coordination.k8s.io",
				},
			},
		},
	}

	found := &rbacv1.ClusterRole{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: resRoleName}, found)
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

func createResourceRoleBinding(c client.Client, namespace string, name string) error {
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
				Kind:      "Group",
				Name:      fmt.Sprintf("system:open-cluster-management:cluster:%s:addon:%s", name, addonName),
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

func deleteRolebindings(c client.Client, namespace string) error {
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
	for idx := range roleList.Items {
		role := roleList.Items[idx]
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

func createRolebindings(c client.Client, namespace string, name string) error {
	err := createClusterRoleBinding(c, namespace, name)
	if err != nil {
		return err
	}
	err = createResourceRoleBinding(c, namespace, name)
	return err
}
