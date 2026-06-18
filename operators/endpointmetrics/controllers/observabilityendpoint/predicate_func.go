// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"fmt"
	"reflect"
	"strings"

	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func getPred(name string, namespace string,
	create bool, update bool, isDelete bool,
) predicate.Funcs {
	createFunc := func(e event.CreateEvent) bool {
		return false
	}
	updateFunc := func(e event.UpdateEvent) bool {
		return false
	}
	deleteFunc := func(e event.DeleteEvent) bool {
		return false
	}
	if create {
		createFunc = func(e event.CreateEvent) bool {
			if e.Object.GetName() == name && (namespace == "" || e.Object.GetNamespace() == namespace) {
				return true
			}
			return false
		}
	}
	if update {
		updateFunc = func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == name &&
				(namespace == "" || e.ObjectNew.GetNamespace() == namespace) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// also check objectNew string in case Kind is empty
				switch {
				case strings.HasPrefix(fmt.Sprint(e.ObjectNew), "&Deployment") ||
					e.ObjectNew.GetObjectKind().GroupVersionKind().Kind == "Deployment":
					if !reflect.DeepEqual(e.ObjectNew.(*appsv1.Deployment).Spec.Template.Spec,
						e.ObjectOld.(*appsv1.Deployment).Spec.Template.Spec) {
						return true
					}
				case e.ObjectNew.GetName() == obAddonName ||
					e.ObjectNew.GetObjectKind().GroupVersionKind().Kind == "ObservabilityAddon":
					specChanged := !reflect.DeepEqual(e.ObjectNew.(*oav1beta1.ObservabilityAddon).Spec,
						e.ObjectOld.(*oav1beta1.ObservabilityAddon).Spec)
					delChanged := e.ObjectNew.GetDeletionTimestamp() != e.ObjectOld.GetDeletionTimestamp()
					if specChanged || delChanged {
						return true
					}
				default:
					return true
				}
			}
			return false
		}
	}
	if isDelete {
		deleteFunc = func(e event.DeleteEvent) bool {
			if e.Object.GetName() == name && (namespace == "" || e.Object.GetNamespace() == namespace) {
				return true
			}
			return false
		}
	}
	return predicate.Funcs{
		CreateFunc: createFunc,
		UpdateFunc: updateFunc,
		DeleteFunc: deleteFunc,
	}
}

// DataChangedPredicate builder for resources where we only care about Data changes (ConfigMaps, Secrets).
func DataChangedPredicate[T client.Object](name, namespace string, getData func(T) any) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj, okOld := e.ObjectOld.(T)
			newObj, okNew := e.ObjectNew.(T)
			// Ensure objects are non-nil to avoid panics on method calls (GetName, GetNamespace)
			if !okOld || !okNew || reflect.ValueOf(oldObj).IsNil() || reflect.ValueOf(newObj).IsNil() {
				return false
			}

			if (name != "" && newObj.GetName() != name) || (namespace != "" && newObj.GetNamespace() != namespace) {
				return false
			}

			return !reflect.DeepEqual(getData(oldObj), getData(newObj))
		},
		CreateFunc: func(e event.CreateEvent) bool {
			obj, ok := e.Object.(T)
			return ok && !reflect.ValueOf(obj).IsNil() && (name == "" || obj.GetName() == name) && (namespace == "" || obj.GetNamespace() == namespace)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			obj, ok := e.Object.(T)
			return ok && !reflect.ValueOf(obj).IsNil() && (name == "" || obj.GetName() == name) && (namespace == "" || obj.GetNamespace() == namespace)
		},
	}
}

// ConfigMapDataChangedPredicate returns a predicate that filters for ConfigMap data changes.
func ConfigMapDataChangedPredicate(name, namespace string) predicate.Funcs {
	return DataChangedPredicate(name, namespace, func(cm *corev1.ConfigMap) any {
		return cm.Data
	})
}

// SecretDataChangedPredicate returns a predicate that filters for Secret data changes.
func SecretDataChangedPredicate(name, namespace string) predicate.Funcs {
	return DataChangedPredicate(name, namespace, func(s *corev1.Secret) any {
		return s.Data
	})
}
