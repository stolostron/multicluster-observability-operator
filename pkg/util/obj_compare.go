// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"reflect"
	"strings"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type compFn func(runtime.Object, runtime.Object) bool

var compFns = map[string]compFn{
	"Namespace":          compareNamespaces,
	"Deployment":         compareDeployments,
	"ServiceAccount":     compareServiceAccounts,
	"ClusterRole":        compareClusterRoles,
	"ClusterRoleBinding": compareClusterRoleBindings,
	"Secret":             compareSecrets,
}

// GetK8sObj is used to get k8s struct based on the passed-in Kind name
func GetK8sObj(kind string) runtime.Object {
	objs := map[string]runtime.Object{
		"Namespace":             &corev1.Namespace{},
		"Deployment":            &v1.Deployment{},
		"ClusterRole":           &rbacv1.ClusterRole{},
		"ClusterRoleBinding":    &rbacv1.ClusterRoleBinding{},
		"ServiceAccount":        &corev1.ServiceAccount{},
		"PersistentVolumeClaim": &corev1.PersistentVolumeClaim{},
		"Secret":                &corev1.Secret{},
	}
	return objs[kind]
}

// CompareObject is used to compare two k8s objs are same or not
func CompareObject(re1 runtime.RawExtension, re2 runtime.RawExtension) bool {
	obj1, err := getObject(re1)
	if err != nil {
		return false
	}
	obj2, err := getObject(re2)
	if err != nil {
		return false
	}
	kind1 := obj1.GetObjectKind().GroupVersionKind().Kind
	kind2 := obj2.GetObjectKind().GroupVersionKind().Kind
	if kind1 != kind2 {
		log.Info("obj1 and obj2 have differnt Kind", "kind1", kind2, "kind2", kind2)
		return false
	}
	return compFns[kind1](obj1, obj2)
}

func getObject(re runtime.RawExtension) (runtime.Object, error) {
	if re.Object != nil {
		return re.Object, nil
	}
	versions := &runtime.VersionedObjects{}
	_, gvk, err := unstructured.UnstructuredJSONScheme.Decode(re.Raw, nil, versions)
	if err != nil {
		log.Error(err, "Failed to decode the raw")
		return nil, err
	}
	obj := GetK8sObj(gvk.Kind)
	err = yaml.NewYAMLOrJSONDecoder(strings.NewReader(string(re.Raw)), 100).Decode(obj)
	if err != nil {
		log.Error(err, "Failed to decode the raw to Kind", "kind", gvk.Kind)
		return nil, err
	}
	return obj, nil
}

func compareNamespaces(obj1 runtime.Object, obj2 runtime.Object) bool {
	ns1 := obj1.(*corev1.Namespace)
	ns2 := obj2.(*corev1.Namespace)
	if ns1.Name != ns2.Name {
		log.Info("Find updated namespace in manifestwork", "namespace1", ns1, "namespace2", ns2)
		return false
	}
	return true
}

func compareDeployments(obj1 runtime.Object, obj2 runtime.Object) bool {
	dep1 := obj1.(*v1.Deployment)
	dep2 := obj2.(*v1.Deployment)
	if dep1.Name != dep2.Name || dep1.Namespace != dep2.Namespace {
		log.Info("Find updated name/namespace for deployment", "deployment", dep1.Name)
		return false
	}
	if !reflect.DeepEqual(dep1.Spec, dep2.Spec) {
		log.Info("Find updated deployment", "deployment", dep1.Name)
		return false
	}
	return true
}

func compareServiceAccounts(obj1 runtime.Object, obj2 runtime.Object) bool {
	sa1 := obj1.(*corev1.ServiceAccount)
	sa2 := obj2.(*corev1.ServiceAccount)
	if sa1.Name != sa2.Name || sa1.Namespace != sa2.Namespace {
		log.Info("Find updated name/namespace for serviceaccount", "serviceaccount", sa1.Name)
		return false
	}
	if !reflect.DeepEqual(sa1.ImagePullSecrets, sa2.ImagePullSecrets) {
		log.Info("Find updated imagepullsecrets in serviceaccount", "serviceaccount", sa1.Name)
		return false
	}
	return true
}

func compareClusterRoles(obj1 runtime.Object, obj2 runtime.Object) bool {
	cr1 := obj1.(*rbacv1.ClusterRole)
	cr2 := obj2.(*rbacv1.ClusterRole)
	if cr1.Name != cr2.Name {
		log.Info("Find updated name for clusterrole", "clusterrole", cr1.Name)
		return false
	}
	if !reflect.DeepEqual(cr1.Rules, cr2.Rules) {
		log.Info("Find updated rules in clusterrole", "clusterrole", cr1.Name)
		return false
	}
	return true
}

func compareClusterRoleBindings(obj1 runtime.Object, obj2 runtime.Object) bool {
	crb1 := obj1.(*rbacv1.ClusterRoleBinding)
	crb2 := obj2.(*rbacv1.ClusterRoleBinding)
	if crb1.Name != crb2.Name {
		log.Info("Find updated name/namespace for clusterrolebinding", "clusterrolebinding", crb1.Name)
		return false
	}
	if !reflect.DeepEqual(crb1.Subjects, crb2.Subjects) || !reflect.DeepEqual(crb1.RoleRef, crb2.RoleRef) {
		log.Info("Find updated subjects/rolerefs for clusterrolebinding", "clusterrolebinding", crb1.Name)
		return false
	}
	return true
}

func compareSecrets(obj1 runtime.Object, obj2 runtime.Object) bool {
	s1 := obj1.(*corev1.Secret)
	s2 := obj2.(*corev1.Secret)
	if s1.Name != s2.Name || s1.Namespace != s2.Namespace {
		log.Info("Find updated name/namespace for secret", "secret", s1.Name)
		return false
	}
	if !reflect.DeepEqual(s1.Data, s2.Data) {
		log.Info("Find updated data in secret", "secret", s1.Name)
		return false
	}
	return true
}
