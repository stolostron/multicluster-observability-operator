// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package deploying

import (
	"context"
	"testing"

	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	replicas1 int32 = 1
	replicas2 int32 = 2
)

func TestDeploy(t *testing.T) {

	cases := []struct {
		name            string
		createObj       runtime.Object
		updateObj       runtime.Object
		validateResults func(client client.Client)
		expectedErr     string
	}{
		{
			name: "create and update the deployment",
			createObj: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "ns1",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas1,
				},
			},
			updateObj: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-deployment",
					Namespace:       "ns1",
					ResourceVersion: "1",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &(replicas2),
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-deployment",
					Namespace: "ns1",
				}
				obj := &appsv1.Deployment{}
				client.Get(context.Background(), namespacedName, obj)

				if *obj.Spec.Replicas != 2 {
					t.Fatalf("fail to update the deployment")
				}
			},
		},
		{
			name: "create and no update the deployment",
			createObj: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment-2",
					Namespace: "ns1",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas1,
				},
			},
			updateObj: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment-2",
					Namespace: "ns1",
					Labels: map[string]string{
						"test-label": "label-value",
					},
					ResourceVersion: "1",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas1,
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-deployment-2",
					Namespace: "ns1",
				}
				obj := &appsv1.Deployment{}
				client.Get(context.Background(), namespacedName, obj)

				if len(obj.ObjectMeta.GetLabels()) != 0 {
					t.Fatalf("should not update the deployment")
				}
			},
		},
		{
			name: "create and update the statefulset",
			createObj: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-statefulSet",
					Namespace: "ns1",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &replicas1,
				},
			},
			updateObj: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-statefulSet",
					Namespace:       "ns1",
					ResourceVersion: "1",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &(replicas2),
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-statefulSet",
					Namespace: "ns1",
				}
				obj := &appsv1.StatefulSet{}
				client.Get(context.Background(), namespacedName, obj)

				if *obj.Spec.Replicas != 2 {
					t.Fatalf("fail to update the statefulset")
				}
			},
		},
		{
			name: "create and update the configmap",
			createObj: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: "ns2",
				},
				Data: map[string]string{
					"test-key": "test-value-1",
				},
			},
			updateObj: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-cm",
					Namespace:       "ns2",
					ResourceVersion: "1",
				},
				Data: map[string]string{
					"test-key": "test-value-2",
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-cm",
					Namespace: "ns2",
				}
				obj := &corev1.ConfigMap{}
				client.Get(context.Background(), namespacedName, obj)

				if obj.Data["test-key"] != "test-value-2" {
					t.Fatalf("fail to update the configmap")
				}
			},
		},
		{
			name: "create and update the service",
			createObj: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "ns2",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"app": "myApp-1",
					},
				},
			},
			updateObj: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-svc",
					Namespace:       "ns2",
					ResourceVersion: "1",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"app": "myApp-2",
					},
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-svc",
					Namespace: "ns2",
				}
				obj := &corev1.Service{}
				client.Get(context.Background(), namespacedName, obj)

				if obj.Spec.Selector["app"] != "myApp-2" {
					t.Fatalf("fail to update the service")
				}
			},
		},
		{
			name: "create and update the secret",
			createObj: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "ns2",
				},
			},
			updateObj: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-secret",
					Namespace:       "ns2",
					ResourceVersion: "1",
				},
				Data: map[string][]byte{
					"username": []byte("YWRtaW4="),
					"password": []byte("MWYyZDFlMmU2N2Rm"),
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-secret",
					Namespace: "ns2",
				}
				obj := &corev1.Secret{}
				client.Get(context.Background(), namespacedName, obj)

				if len(obj.Data) == 0 {
					t.Fatalf("fail to update the secret")
				}
			},
		},
		{
			name: "create and update the clusterrole",
			createObj: &rbacv1.ClusterRole{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rbac.authorization.k8s.io/v1",
					Kind:       "ClusterRole",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-clusterrole",
				},
			},
			updateObj: &rbacv1.ClusterRole{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rbac.authorization.k8s.io/v1",
					Kind:       "ClusterRole",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-clusterrole",
					ResourceVersion: "1",
				},
				Rules: []rbacv1.PolicyRule{
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
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name: "test-clusterrole",
				}
				obj := &rbacv1.ClusterRole{}
				client.Get(context.Background(), namespacedName, obj)

				if len(obj.Rules) == 0 {
					t.Fatalf("fail to update the clusterrole")
				}
			},
		},
		{
			name: "create and update the clusterrolebinding",
			createObj: &rbacv1.ClusterRoleBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rbac.authorization.k8s.io/v1",
					Kind:       "ClusterRoleBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-clusterrolebinding",
				},
			},
			updateObj: &rbacv1.ClusterRoleBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rbac.authorization.k8s.io/v1",
					Kind:       "ClusterRoleBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-clusterrolebinding",
					ResourceVersion: "1",
				},
				RoleRef: rbacv1.RoleRef{
					Kind:     "Role",
					Name:     "test-clusterrole",
					APIGroup: "rbac.authorization.k8s.io",
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "test-sa",
						Namespace: "ns2",
					},
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name: "test-clusterrolebinding",
				}
				obj := &rbacv1.ClusterRoleBinding{}
				client.Get(context.Background(), namespacedName, obj)

				if len(obj.Subjects) == 0 {
					t.Fatalf("fail to update the clusterrolebinding")
				}
			},
		},
		{
			name: "serviceaccount update is not supported",
			createObj: &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ServiceAccount",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "ns1",
				},
			},
			updateObj: &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ServiceAccount",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "ns1",
					Labels: map[string]string{
						"test-label": "label-value",
					},
					ResourceVersion: "1",
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-sa",
					Namespace: "ns1",
				}
				obj := &corev1.ServiceAccount{}
				client.Get(context.Background(), namespacedName, obj)

				if len(obj.GetObjectMeta().GetLabels()) != 0 {
					t.Fatalf("update serviceaccount is not supported then")
				}
			},
		},
		{
			name: "create and update the prometheus",
			createObj: &prometheusv1.Prometheus{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       "Prometheus",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-prometheus",
					Namespace: "ns1",
				},
				Spec: prometheusv1.PrometheusSpec{
					AdditionalAlertManagerConfigs: &corev1.SecretKeySelector{
						Key: "old",
					},
				},
			},
			updateObj: &prometheusv1.Prometheus{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "monitoring.coreos.com/v1",
					Kind:       "Prometheus",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-prometheus",
					Namespace:       "ns1",
					ResourceVersion: "1",
				},
				Spec: prometheusv1.PrometheusSpec{
					AdditionalAlertManagerConfigs: &corev1.SecretKeySelector{
						Key: "new",
					},
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-prometheus",
					Namespace: "ns1",
				}
				obj := &prometheusv1.Prometheus{}
				client.Get(context.Background(), namespacedName, obj)

				if obj.Spec.AdditionalAlertManagerConfigs.Key != "new" {
					t.Fatalf("fail to update the prometheus")
				}
			},
		},
		{
			name: "create and update an ingress",
			createObj: &networkingv1.Ingress{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "networking.k8s.io/v1",
					Kind:       "Ingress",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "ns1",
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: toPtr(t, "test-class"),
				},
			},
			updateObj: &networkingv1.Ingress{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "networking.k8s.io/v1",
					Kind:       "Ingress",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-ingress",
					Namespace:       "ns1",
					ResourceVersion: "1",
					Annotations: map[string]string{
						"test-annotation": "test-value",
					},
				},
				Spec: networkingv1.IngressSpec{
					IngressClassName: toPtr(t, "test-class-changed"),
					TLS: []networkingv1.IngressTLS{
						{
							Hosts:      []string{"test-host"},
							SecretName: "test",
						},
					},
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-ingress",
					Namespace: "ns1",
				}
				obj := &networkingv1.Ingress{}
				client.Get(context.Background(), namespacedName, obj)

				if len(obj.Spec.TLS) != 1 {
					t.Fatalf("fail to update the ingress")
				}

				if obj.Spec.TLS[0].SecretName != "test" {
					t.Fatalf("fail to update the ingress")
				}

				if *obj.Spec.IngressClassName != "test-class-changed" {
					t.Fatalf("fail to update the ingress")
				}
				if obj.Annotations["test-annotation"] != "test-value" {
					t.Fatalf("fail to update the ingress")
				}
			},
		},
		{
			name: "create and update a role",
			createObj: &rbacv1.Role{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rbac.authorization.k8s.io/v1",
					Kind:       "Role",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-role",
					Namespace: "ns1",
				},
				Rules: []rbacv1.PolicyRule{
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
			},
			updateObj: &rbacv1.Role{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rbac.authorization.k8s.io/v1",
					Kind:       "Role",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-role",
					Namespace:       "ns1",
					ResourceVersion: "1",
				},
				Rules: []rbacv1.PolicyRule{
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
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-role",
					Namespace: "ns1",
				}
				obj := &rbacv1.Role{}
				client.Get(context.Background(), namespacedName, obj)

				if len(obj.Rules[0].Verbs) != 3 {
					t.Fatalf("fail to update the role")
				}
			},
		},
		{
			name: "create and update a rolebinding",
			createObj: &rbacv1.RoleBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rbac.authorization.k8s.io/v1",
					Kind:       "RoleBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rolebinding",
					Namespace: "ns1",
				},
				RoleRef: rbacv1.RoleRef{
					Kind:     "Role",
					Name:     "test-role",
					APIGroup: "rbac.authorization.k8s.io",
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "test-sa",
						Namespace: "ns1",
					},
				},
			},
			updateObj: &rbacv1.RoleBinding{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rbac.authorization.k8s.io/v1",
					Kind:       "RoleBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-rolebinding",
					Namespace:       "ns1",
					ResourceVersion: "1",
				},
				RoleRef: rbacv1.RoleRef{
					Kind:     "Role",
					Name:     "test-role",
					APIGroup: "rbac.authorization.k8s.io",
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "test-sa",
						Namespace: "ns1",
					},
					{
						Kind:      "User",
						Name:      "test-user",
						Namespace: "ns1",
					},
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-rolebinding",
					Namespace: "ns1",
				}
				obj := &rbacv1.RoleBinding{}
				client.Get(context.Background(), namespacedName, obj)

				if len(obj.Subjects) != 2 {
					t.Fatalf("fail to update the rolebinding")
				}
			},
		},
		{
			name: "create and update serviceaccount",
			createObj: &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ServiceAccount",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "ns1",
				},
			},
			updateObj: &corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ServiceAccount",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-sa",
					Namespace:       "ns1",
					ResourceVersion: "1",
				},
				ImagePullSecrets: []corev1.LocalObjectReference{
					{
						Name: "test-secret",
					},
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-sa",
					Namespace: "ns1",
				}
				obj := &corev1.ServiceAccount{}
				client.Get(context.Background(), namespacedName, obj)

				if len(obj.ImagePullSecrets) == 0 {
					t.Fatalf("fail to update the serviceaccount")
				}
			},
		},
		{
			name: "create and update daemonset",
			createObj: &appsv1.DaemonSet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "DaemonSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-daemonset",
					Namespace: "ns1",
				},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "myApp",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "myApp",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "test-image",
								},
							},
						},
					},
				},
			},
			updateObj: &appsv1.DaemonSet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "DaemonSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-daemonset",
					Namespace:       "ns1",
					ResourceVersion: "1",
				},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "myApp",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "myApp",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-container",
									Image: "test-image:latest",
								},
							},
						},
					},
				},
			},
			validateResults: func(client client.Client) {
				namespacedName := types.NamespacedName{
					Name:      "test-daemonset",
					Namespace: "ns1",
				}
				obj := &appsv1.DaemonSet{}
				client.Get(context.Background(), namespacedName, obj)

				if obj.Spec.Template.Spec.Containers[0].Image != "test-image:latest" {
					t.Fatalf("fail to update the daemonset")
				}
			},
		},
	}

	scheme := runtime.NewScheme()

	corev1.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)
	rbacv1.AddToScheme(scheme)
	prometheusv1.AddToScheme(scheme)
	networkingv1.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	deployer := NewDeployer(client)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			createObjUns, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(c.createObj)
			err := deployer.Deploy(context.Background(), &unstructured.Unstructured{Object: createObjUns})
			if err != nil {
				t.Fatalf("Cannot create the resource %v", err)
			}
			if c.updateObj != nil {
				updateObjUns, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(c.updateObj)
				err = deployer.Deploy(context.Background(), &unstructured.Unstructured{Object: updateObjUns})
				if err != nil {
					t.Fatalf("Cannot update the resource %v", err)
				}
			}

			c.validateResults(client)
		})
	}

}

func toPtr(t *testing.T, s string) *string {
	t.Helper()
	return &s
}
