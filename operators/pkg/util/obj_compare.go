// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"reflect"
	"strings"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	mcov1beta1 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

type compFn func(runtime.Object, runtime.Object) bool

var log = logf.Log.WithName("obj_compare")

var compFns = map[string]compFn{
	"Namespace":                compareNamespaces,
	"Deployment":               compareDeployments,
	"ServiceAccount":           compareServiceAccounts,
	"ClusterRole":              compareClusterRoles,
	"ClusterRoleBinding":       compareClusterRoleBindings,
	"Secret":                   compareSecrets,
	"Service":                  compareServices,
	"ConfigMap":                compareConfigMap,
	"CustomResourceDefinition": compareCRD,
	"ObservabilityAddon":       compareObsAddon,
}

// GetK8sObj is used to get k8s struct based on the passed-in Kind name
func GetK8sObj(kind string) runtime.Object {
	objs := map[string]runtime.Object{
		"Namespace":                &corev1.Namespace{},
		"Deployment":               &v1.Deployment{},
		"StatefulSet":              &v1.StatefulSet{},
		"DaemonSet":                &v1.DaemonSet{},
		"ClusterRole":              &rbacv1.ClusterRole{},
		"ClusterRoleBinding":       &rbacv1.ClusterRoleBinding{},
		"ServiceAccount":           &corev1.ServiceAccount{},
		"PersistentVolumeClaim":    &corev1.PersistentVolumeClaim{},
		"Secret":                   &corev1.Secret{},
		"ConfigMap":                &corev1.ConfigMap{},
		"Service":                  &corev1.Service{},
		"CustomResourceDefinition": &apiextensionsv1.CustomResourceDefinition{},
		"ObservabilityAddon":       &mcov1beta1.ObservabilityAddon{},
	}
	return objs[kind]
}

// CompareObject is used to compare two k8s objs are same or not
func CompareObject(re1 runtime.RawExtension, re2 runtime.RawExtension) bool {
	if re2.Object == nil {
		return reflect.DeepEqual(re1.Raw, re2.Raw)
	}
	obj1, err := GetObject(re1)
	if err != nil {
		return false
	}
	obj2, err := GetObject(re2)
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

func GetObject(re runtime.RawExtension) (runtime.Object, error) {
	if re.Object != nil {
		return re.Object, nil
	}
	_, gvk, err := unstructured.UnstructuredJSONScheme.Decode(re.Raw, nil, re.Object)
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

func compareServices(obj1 runtime.Object, obj2 runtime.Object) bool {
	s1 := obj1.(*corev1.Service)
	s2 := obj2.(*corev1.Service)
	if s1.Name != s2.Name || s1.Namespace != s2.Namespace {
		log.Info("Find updated name/namespace for service", "service", s1.Name)
		return false
	}
	if !reflect.DeepEqual(s1.Spec, s2.Spec) {
		log.Info("Find updated data in service", "service", s1.Name)
		return false
	}
	return true
}

func compareConfigMap(obj1 runtime.Object, obj2 runtime.Object) bool {
	cm1 := obj1.(*corev1.ConfigMap)
	cm2 := obj2.(*corev1.ConfigMap)
	if cm1.Name != cm2.Name || cm1.Namespace != cm2.Namespace {
		log.Info("Find updated name/namespace for configmap", "configmap", cm1.Name)
		return false
	}
	if !reflect.DeepEqual(cm1.Data, cm2.Data) {
		log.Info("Find updated data in secret", "secret", cm1.Name)
		return false
	}
	return true
}

func compareCRD(obj1 runtime.Object, obj2 runtime.Object) bool {
	crd1 := obj1.(*apiextensionsv1.CustomResourceDefinition)
	crd2 := obj2.(*apiextensionsv1.CustomResourceDefinition)
	if crd1.Name != crd2.Name {
		log.Info("Find updated name for crd", "crd", crd1.Name)
		return false
	}
	if !reflect.DeepEqual(crd1.Spec, crd2.Spec) {
		log.Info("Find updated spec for crd", "crd", crd1.Name)
		return false
	}
	return true
}

func compareObsAddon(obj1 runtime.Object, obj2 runtime.Object) bool {
	return reflect.DeepEqual(obj1, obj2)
}
