// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	replicas1 int32 = 1
	replicas2 int32 = 2
)

func TestCompareObject(t *testing.T) {
	cases := []struct {
		name            string
		rawObj1         runtime.RawExtension
		rawObj2         runtime.RawExtension
		rawObj3         runtime.RawExtension
		validateResults func(re1, re2 runtime.RawExtension)
	}{
		{
			name: "Compare namespaces",
			rawObj1: runtime.RawExtension{
				Raw: []byte(`{
	"apiVersion": "v1",
	"kind": "Namespace",
	"metadata": {
		"name": "test-ns-1"
	}
}`),
			},
			rawObj2: runtime.RawExtension{
				Object: &corev1.Namespace{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Namespace",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-ns-2",
					},
					Spec: corev1.NamespaceSpec{},
				},
			},
		},
		{
			name: "Compare serviceaccount",
			rawObj1: runtime.RawExtension{
				Object: &corev1.ServiceAccount{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "ServiceAccount",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sa-1",
						Namespace: "ns1",
					},
					ImagePullSecrets: []corev1.LocalObjectReference{
						{
							Name: "test-image-pull-secret-1",
						},
					},
				},
			},
			rawObj2: runtime.RawExtension{
				Object: &corev1.ServiceAccount{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "ServiceAccount",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sa-2",
						Namespace: "ns1",
					},
				},
			},
			rawObj3: runtime.RawExtension{
				Object: &corev1.ServiceAccount{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "ServiceAccount",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sa-1",
						Namespace: "ns1",
					},
					ImagePullSecrets: []corev1.LocalObjectReference{
						{
							Name: "test-image-pull-secret-3",
						},
					},
				},
			},
		},
		{
			name: "Compare ClusterRoleBinding",
			rawObj1: runtime.RawExtension{
				Object: &rbacv1.ClusterRoleBinding{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "rbac.authorization.k8s.io/v1",
						Kind:       "ClusterRoleBinding",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-clusterrolebinding-1",
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "Role",
						Name:     "test-clusterrole-1",
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
			},
			rawObj2: runtime.RawExtension{
				Object: &rbacv1.ClusterRoleBinding{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "rbac.authorization.k8s.io/v1",
						Kind:       "ClusterRoleBinding",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-clusterrolebinding-2",
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "Role",
						Name:     "test-clusterrole-2",
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
			},
			rawObj3: runtime.RawExtension{
				Object: &rbacv1.ClusterRoleBinding{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "rbac.authorization.k8s.io/v1",
						Kind:       "ClusterRoleBinding",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-clusterrolebinding-1",
					},
					RoleRef: rbacv1.RoleRef{
						Kind:     "Role",
						Name:     "test-clusterrole-2",
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
			},
		},
		{
			name: "Compare ClusterRole",
			rawObj1: runtime.RawExtension{
				Object: &rbacv1.ClusterRole{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "rbac.authorization.k8s.io/v1",
						Kind:       "ClusterRole",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-clusterrole-1",
					},
				},
			},
			rawObj2: runtime.RawExtension{
				Object: &rbacv1.ClusterRole{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "rbac.authorization.k8s.io/v1",
						Kind:       "ClusterRole",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-clusterrole-2",
					},
				},
			},
			rawObj3: runtime.RawExtension{
				Object: &rbacv1.ClusterRole{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "rbac.authorization.k8s.io/v1",
						Kind:       "ClusterRole",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-clusterrole-1",
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
			},
		},
		{
			name: "Compare Deployment",
			rawObj1: runtime.RawExtension{
				Object: &appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment-1",
						Namespace: "ns1",
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas1,
					},
				},
			},
			rawObj2: runtime.RawExtension{
				Object: &appsv1.Deployment{
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
			},
			rawObj3: runtime.RawExtension{
				Object: &appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment-1",
						Namespace: "ns1",
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas2,
					},
				},
			},
		},
		{
			name: "Compare Secret",
			rawObj1: runtime.RawExtension{
				Object: &corev1.Secret{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Secret",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret-1",
						Namespace: "ns2",
					},
				},
			},
			rawObj2: runtime.RawExtension{
				Object: &corev1.Secret{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Secret",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret-2",
						Namespace: "ns2",
					},
				},
			},
			rawObj3: runtime.RawExtension{
				Object: &corev1.Secret{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Secret",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret-1",
						Namespace: "ns2",
					},
					Data: map[string][]byte{
						"username": []byte("YWRtaW4="),
						"password": []byte("MWYyZDFlMmU2N2Rm"),
					},
				},
			},
		},
		{
			name: "Compare CRD",
			rawObj1: runtime.RawExtension{
				Object: &apiextensionsv1.CustomResourceDefinition{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "CustomResourceDefinition",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-crd-1",
					},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Group: "group1",
					},
				},
			},
			rawObj2: runtime.RawExtension{
				Object: &apiextensionsv1beta1.CustomResourceDefinition{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1beta1",
						Kind:       "CustomResourceDefinition",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-crd-1",
					},
					Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{},
				},
			},
			rawObj3: runtime.RawExtension{
				Object: &apiextensionsv1.CustomResourceDefinition{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "CustomResourceDefinition",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-crd-1",
					},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Group: "group2",
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if CompareObject(c.rawObj1, c.rawObj2) {
				t.Errorf("The two objects are same. Actually they should be different.")
			}
			if !CompareObject(c.rawObj1, c.rawObj1) {
				t.Errorf("The same object should be no difference.")
			}
			if CompareObject(c.rawObj1, c.rawObj3) {
				t.Errorf("The object may not be updated.")
			}
		})
	}
}
