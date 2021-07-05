// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package deploying

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	}

	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)
	rbacv1.AddToScheme(scheme)
	client := fake.NewFakeClientWithScheme(scheme, []runtime.Object{}...)

	deployer := NewDeployer(client)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			createObjUns, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(c.createObj)
			err := deployer.Deploy(&unstructured.Unstructured{Object: createObjUns})
			if err != nil {
				t.Fatalf("Cannot create the deployment %v", err)
			}
			if c.updateObj != nil {
				updateObjUns, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(c.updateObj)
				err = deployer.Deploy(&unstructured.Unstructured{Object: updateObjUns})
				if err != nil {
					t.Fatalf("Cannot update the deployment %v", err)
				}
			}

			c.validateResults(client)
		})
	}

}
