// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package observabilityendpoint

import (
	"fmt"
	"reflect"
	"strings"

	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func getPred(name string, namespace string,
	create bool, update bool, isDelete bool) predicate.Funcs {
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
					if !reflect.DeepEqual(e.ObjectNew.(*v1.Deployment).Spec.Template.Spec,
						e.ObjectOld.(*v1.Deployment).Spec.Template.Spec) {
						return true
					}
				case e.ObjectNew.GetName() == obAddonName ||
					e.ObjectNew.GetObjectKind().GroupVersionKind().Kind == "ObservabilityAddon":
					if !reflect.DeepEqual(e.ObjectNew.(*oav1beta1.ObservabilityAddon).Spec,
						e.ObjectOld.(*oav1beta1.ObservabilityAddon).Spec) {
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

func configMapDataChangedPredicate(name, namespace string) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCM, okOld := e.ObjectOld.(*corev1.ConfigMap)
			newCM, okNew := e.ObjectNew.(*corev1.ConfigMap)
			if !okOld || !okNew {
				return false
			}

			if newCM.Name != name || newCM.Namespace != namespace {
				return false
			}

			return !reflect.DeepEqual(oldCM.Data, newCM.Data)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			cm, ok := e.Object.(*corev1.ConfigMap)
			return ok && cm.Name == name && cm.Namespace == namespace
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			cm, ok := e.Object.(*corev1.ConfigMap)
			return ok && cm.Name == name && cm.Namespace == namespace
		},
	}
}
