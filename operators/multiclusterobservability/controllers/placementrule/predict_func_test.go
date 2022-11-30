// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	workv1 "open-cluster-management.io/api/work/v1"
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
		ObjectNew: newClusterMgmtAddon(),
		ObjectOld: newAddon,
	}
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile triggered for clustermanagementaddon update event when no supportedConfigs updated")
	}

	ue.ObjectOld.SetName("update_name")
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile triggered for clustermanagementaddon update event when no supportedConfigs updated")
	}

	newAddon.Spec.SupportedConfigs[0].DefaultConfig.Name = "update_name"
	ue = event.UpdateEvent{
		ObjectOld: newClusterMgmtAddon(),
		ObjectNew: newAddon,
	}
	if !pred.UpdateFunc(ue) {
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
		ObjectNew: newManagedClusterAddon(),
		ObjectOld: newAddon,
	}
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile triggered for managedclusteraddon update event when no supportedConfigs updated")
	}

	ue.ObjectOld.SetName("update_name")
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile triggered for managedclusteraddon update event when no supportedConfigs updated")
	}

	newAddon.Spec.Configs[0].ConfigReferent.Name = "update_name"
	ue = event.UpdateEvent{
		ObjectOld: newManagedClusterAddon(),
		ObjectNew: newAddon,
	}
	if !pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered for managedclusteraddon when supportedConfigs updated")
	}
}

func TestGetManifestworkPredFunc(t *testing.T) {
	pred := getManifestworkPred()
	work := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observability-work",
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
	}

	ce := event.CreateEvent{
		Object: work,
	}
	if pred.CreateFunc(ce) {
		t.Fatal("reconcile triggered for manifestwork create event")
	}

	de := event.DeleteEvent{
		Object: work,
	}
	if !pred.DeleteFunc(de) {
		t.Fatal("reconcile not triggered for managedclusteraddon delete event")
	}

	newWork := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "observability-work",
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{
						RawExtension: runtime.RawExtension{
							Object: newManagedClusterAddon(),
						},
					},
				},
			},
		},
	}
	ue := event.UpdateEvent{
		ObjectNew: newWork,
		ObjectOld: work,
	}
	if !pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered for managedclusteraddon update event")
	}

}
