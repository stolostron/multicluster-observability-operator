// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestGetClusterMgmtAddonPredFunc(t *testing.T) {
	pred := getClusterMgmtAddonPredFunc()
	ce := event.CreateEvent{
		Object: newClusterMgmtAddon(),
	}
	if pred.CreateFunc(ce) {
		t.Fatal("reconcile triggered for clustermanagementaddon create event")
	}

	de := event.DeleteEvent{
		Object: newClusterMgmtAddon(),
	}
	if pred.DeleteFunc(de) {
		t.Fatal("reconcile triggered for clustermanagementaddon delete event")
	}

	newAddon := newClusterMgmtAddon()
	ue := event.UpdateEvent{
		ObjectOld: newClusterMgmtAddon(),
		ObjectNew: newAddon,
	}
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile triggered for clustermanagementaddon update event when no supportedConfigs updated")
	}

	newAddon.Spec.SupportedConfigs[0].DefaultConfig.Name = "update_name"
	ue = event.UpdateEvent{
		ObjectOld: newClusterMgmtAddon(),
		ObjectNew: newAddon,
	}
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered for clustermanagementaddon when supportedConfigs updated")
	}
}

func TestGetMgClusterAddonPredFunc(t *testing.T) {
	pred := getMgClusterAddonPredFunc()
	ce := event.CreateEvent{
		Object: newManagedClusterAddon(),
	}
	if pred.CreateFunc(ce) {
		t.Fatal("reconcile triggered for managedclusteraddon create event")
	}

	de := event.DeleteEvent{
		Object: newManagedClusterAddon(),
	}
	if pred.DeleteFunc(de) {
		t.Fatal("reconcile triggered for managedclusteraddon delete event")
	}

	newAddon := newManagedClusterAddon()
	ue := event.UpdateEvent{
		ObjectOld: newManagedClusterAddon(),
		ObjectNew: newAddon,
	}
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile triggered for managedclusteraddon update event when no supportedConfigs updated")
	}

	newAddon.Spec.Configs[0].ConfigReferent.Name = "update_name"
	ue = event.UpdateEvent{
		ObjectOld: newManagedClusterAddon(),
		ObjectNew: newAddon,
	}
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered for managedclusteraddon when supportedConfigs updated")
	}
}
