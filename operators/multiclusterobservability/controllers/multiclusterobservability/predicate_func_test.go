// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"testing"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestGetMCOPredicateFunc(t *testing.T) {
	pred := GetMCOPredicateFunc()
	mco := newMultiClusterObservability()
	mco.ObjectMeta.ResourceVersion = "1"
	ce := event.CreateEvent{
		Object: mco,
	}
	if !pred.CreateFunc(ce) {
		t.Fatal("reconcile not triggered for mco create event")
	}
	de := event.DeleteEvent{
		Object:             mco,
		DeleteStateUnknown: false,
	}
	if !pred.DeleteFunc(de) {
		t.Fatal("reconcile not triggered for mco delete event")
	}
	mcoNew := newMultiClusterObservability()
	mcoNew.ObjectMeta.ResourceVersion = "2"
	ue := event.UpdateEvent{
		ObjectOld: mco,
		ObjectNew: mcoNew,
	}
	if !pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered for mco update event")
	}
}

func TestGetConfigMapPredicateFunc(t *testing.T) {
	pred := GetConfigMapPredicateFunc()
	cm := createAlertManagerConfigMap(config.AlertRuleCustomConfigMapName)
	ce := event.CreateEvent{
		Object: cm,
	}
	if !pred.CreateFunc(ce) {
		t.Fatal("reconcile not triggered for configmap create event")
	}
	de := event.DeleteEvent{
		Object: cm,
	}
	if !pred.DeleteFunc(de) {
		t.Fatal("reconcile not triggered for configmap delete event")
	}
	ue := event.UpdateEvent{
		ObjectNew: cm,
	}
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered for configmap update event")
	}
}

func TestGetSecretPredicateFunc(t *testing.T) {
	pred := GetAlertManagerSecretPredicateFunc()
	sec := createSecret("test", config.AlertmanagerRouteBYOCAName, config.GetDefaultNamespace())
	ce := event.CreateEvent{
		Object: sec,
	}
	if !pred.CreateFunc(ce) {
		t.Fatal("reconcile not triggered for secret create event")
	}
	de := event.DeleteEvent{
		Object: sec,
	}
	if !pred.DeleteFunc(de) {
		t.Fatal("reconcile not triggered for secret delete event")
	}
	ue := event.UpdateEvent{
		ObjectNew: sec,
	}
	if !pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered for secret update event")
	}
	t.Log("TODO: implement TestGetSecretPredicateFunc")
}

func TestGetNamespacePredicateFunc(t *testing.T) {
	pred := GetNamespacePredicateFunc()
	ns := createNamespaceInstance("open-cluster-management-observability")
	ce := event.CreateEvent{
		Object: ns,
	}
	if !pred.CreateFunc(ce) {
		t.Fatal("reconcile not triggered for namespace create event")
	}
	de := event.DeleteEvent{
		Object: ns,
	}
	if pred.DeleteFunc(de) {
		t.Fatal("reconcile triggered for namespace delete event")
	}
	ns.ObjectMeta.Labels["openshift.io/cluster-monitoring"] = "false"
	ue := event.UpdateEvent{
		ObjectNew: ns,
	}
	if !pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered for namespace update event")
	}
}
