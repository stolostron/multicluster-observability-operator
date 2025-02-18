// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"fmt"
	"reflect"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

const (
	addonName          = "observability-controller" // #nosec G101 -- Not a hardcoded credential.
	resRoleName        = "endpoint-observability-res-role"
	resRoleBindingName = "endpoint-observability-res-rolebinding"
	mcoRoleName        = "endpoint-observability-mco-role"
	mcoRoleBindingName = "endpoint-observability-mco-rolebinding"
	epRsName           = "observabilityaddons"
	epStatusRsName     = "observabilityaddons/status"
)

// createReadMCOClusterRole creates a role with read permissions on the MCO resource
func createReadMCOClusterRole(ctx context.Context, c client.Client) error {
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
	err := c.Get(ctx, types.NamespacedName{Name: mcoRoleName}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating mco clusterRole")
		err = c.Create(ctx, role)
		if err != nil {
			return fmt.Errorf("failed to create endpoint-observability-mco-role clusterRole: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check endpoint-observability-mco-role clusterRole: %w", err)
	}

	if !reflect.DeepEqual(found.Rules, role.Rules) {
		log.Info("Updating endpoint-observability-mco-role clusterRole")
		role.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = c.Update(ctx, role)
		if err != nil {
			return fmt.Errorf("failed to update endpoint-observability-mco-role clusterRole: %w", err)
		}
		return nil
	}

	return nil
}

func createReadMCOClusterRoleBinding(c client.Client, namespace string, name string) error {
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

	return nil
}

func createResourceRole(ctx context.Context, c client.Client) error {
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
					"managedclusteraddons/finalizers",
				},
				Verbs: []string{
					"watch",
					"list",
					"get",
					"update",
					"patch",
					"delete",
					"create",
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
	err := c.Get(ctx, types.NamespacedName{Name: resRoleName}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating endpoint-observability-res-role clusterrole")
		if err := c.Create(ctx, role); err != nil {
			return fmt.Errorf("failed to create endpoint-observability-res-role clusterrole: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check endpoint-observability-res-role clusterrole: %w", err)
	}

	if !reflect.DeepEqual(found.Rules, role.Rules) {
		log.Info("Updating endpoint-observability-res-role clusterrole")
		role.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		if err := c.Update(ctx, role); err != nil {
			log.Error(err, "Failed to update endpoint-observability-res-role clusterrole")
			return err
		}
		return nil
	}

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
			log.Error(
				err,
				"Failed to create endpoint-observability-res-rolebinding rolebinding",
				"namespace",
				namespace,
			)
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
			log.Error(
				err,
				"Failed to update endpoint-observability-res-rolebinding rolebinding",
				"namespace",
				namespace,
			)
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
			Name: namespace + "-" + mcoRoleBindingName,
		},
	}
	err := c.Delete(context.TODO(), crb)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete clusterrolebinding", "name", namespace+"-"+resRoleBindingName)
			return err
		}
	} else {
		log.Info("Clusterrolebinding deleted", "name", namespace+"-"+resRoleBindingName)
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resRoleBindingName,
			Namespace: namespace,
		},
	}
	err = c.Delete(context.TODO(), rb)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete rolebinding", "name", resRoleBindingName, "namespace", namespace)
			return err
		}
	} else {
		log.Info("Rolebinding deleted", "name", resRoleBindingName, "namespace", namespace)
	}

	return nil
}

func createRolebindings(c client.Client, namespace string, name string) error {
	err := createReadMCOClusterRoleBinding(c, namespace, name)
	if err != nil {
		return err
	}
	err = createResourceRoleBinding(c, namespace, name)
	return err
}
