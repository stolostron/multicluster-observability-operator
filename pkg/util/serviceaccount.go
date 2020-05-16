package util

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RoleName           = "monitoring-endpoint-metrics"
	RoleBindingName    = "monitoring-endpoint-metrics"
	ServiceAccountName = "monitoring-endpoint-metrics-sa"
	ResourceName       = "endpointmetrics"
	ResourceGroup      = "monitoring.open-cluster-management.io"
)

func createRole(client client.Client, namespace string) error {

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RoleName,
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Resources: []string{
					ResourceName,
				},
				Verbs: []string{
					"watch",
					"list",
					"get",
				},
				APIGroups: []string{
					ResourceGroup,
				},
			},
		},
	}

	found := &rbacv1.Role{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: RoleName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating monitoring-endpoint-metrics role", "namespace", namespace)
		err = client.Create(context.TODO(), role)
		if err != nil {
			log.Error(err, "Failed to create monitoring-endpoint-metrics role")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check monitoring-endpoint-metrics role")
		return err
	}

	return nil
}

func createRoleBinding(client client.Client, namespace string) error {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RoleBindingName,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     RoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ServiceAccountName,
				Namespace: namespace,
			},
		},
	}
	found := &rbacv1.RoleBinding{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: RoleBindingName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating monitoring-endpoint-metrics rolebinding", "namespace", namespace)
		err = client.Create(context.TODO(), rb)
		if err != nil {
			log.Error(err, "Failed to create monitoring-endpoint-metrics rolebinding")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check monitoring-endpoint-metrics rolebinding")
		return err
	}

	return nil
}

func createServiceAccount(client client.Client, namespace string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceAccountName,
			Namespace: namespace,
		},
	}
	found := &corev1.ServiceAccount{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: ServiceAccountName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating monitoring-endpoint-metrics-sa serviceaccount", "namespace", namespace)
		err = client.Create(context.TODO(), sa)
		if err != nil {
			log.Error(err, "Failed to create monitoring-endpoint-metrics-sa serviceaccount")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check monitoring-endpoint-metrics-sa serviceaccount")
		return err
	}

	return nil
}

func getSAToken(client client.Client, namespace string) ([]byte, []byte, error) {
	err := createRole(client, namespace)
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
	err = client.Get(context.TODO(), types.NamespacedName{Name: ServiceAccountName, Namespace: namespace}, saFound)
	if err != nil {
		log.Error(err, "Failed to get monitoring-endpoint-metrics-sa serviceaccount", "namespace", namespace)
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
		log.Info("Secret Name", "name", s.Name)
		log.Info("Secret Type", "type", secretFound.Type)
		if secretFound.Type == corev1.SecretTypeServiceAccountToken {
			token := secretFound.Data["token"]
			ca := secretFound.Data["ca.crt"]
			return ca, token, nil
		}
	}
	return nil, nil, errors.NewNotFound(corev1.Resource("secret"), saFound.Name+"-token-*")
}
