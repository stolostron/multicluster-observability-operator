// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"testing"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorutil "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestGetClusterMgmtAddonPredFunc_StatusChanges(t *testing.T) {
	pred := getClusterMgmtAddonPredFunc()

	oldAddon := newClusterMgmtAddon()
	newAddon := newClusterMgmtAddon()

	// Test: Status change without defaultConfigReferences should not trigger reconcile
	ue := event.UpdateEvent{
		ObjectOld: oldAddon,
		ObjectNew: newAddon,
	}
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile triggered when CMA status unchanged")
	}

	// Test: Status change with defaultConfigReferences should trigger reconcile
	newAddon.Status.DefaultConfigReferences = []addonv1alpha1.DefaultConfigReference{
		{
			ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
				Group:    operatorutil.AddonGroup,
				Resource: operatorutil.AddonDeploymentConfigResource,
			},
			DesiredConfig: &addonv1alpha1.ConfigSpecHash{
				ConfigReferent: addonv1alpha1.ConfigReferent{
					Name:      "test-config",
					Namespace: "test-ns",
				},
				SpecHash: "abc123",
			},
		},
	}

	ue = event.UpdateEvent{
		ObjectOld: oldAddon,
		ObjectNew: newAddon,
	}
	if !pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered when CMA status.defaultConfigReferences changed")
	}

	// Test: Status specHash change should trigger reconcile
	newerAddon := newAddon.DeepCopy()
	newerAddon.Status.DefaultConfigReferences[0].DesiredConfig.SpecHash = "xyz789"
	ue = event.UpdateEvent{
		ObjectOld: newAddon,
		ObjectNew: newerAddon,
	}
	if !pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered when CMA status specHash changed")
	}
}

func TestGetMgClusterAddonPredFunc(t *testing.T) {
	pred := getMgClusterAddonPredFunc()
	ce := event.CreateEvent{
		Object: newManagedClusterAddon(),
	}
	if !pred.CreateFunc(ce) {
		t.Fatal("reconcile failed to trigger for managedclusteraddon create event")
	}

	invalidAddon := newManagedClusterAddon()
	invalidAddon.Name = "another-addon"
	if pred.CreateFunc(event.CreateEvent{Object: invalidAddon}) {
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

func TestGetMgClusterAddonPredFunc_StatusChanges(t *testing.T) {
	pred := getMgClusterAddonPredFunc()

	oldAddon := newManagedClusterAddon()
	newAddon := newManagedClusterAddon()

	// Test: Status change without configReferences should not trigger reconcile
	ue := event.UpdateEvent{
		ObjectOld: oldAddon,
		ObjectNew: newAddon,
	}
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile triggered when MCA status unchanged")
	}

	// Test: Status change with configReferences should trigger reconcile
	newAddon.Status.ConfigReferences = []addonv1alpha1.ConfigReference{
		{
			ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
				Group:    operatorutil.AddonGroup,
				Resource: operatorutil.AddonDeploymentConfigResource,
			},
			ConfigReferent: addonv1alpha1.ConfigReferent{
				Name:      "test-config",
				Namespace: "test-ns",
			},
			DesiredConfig: &addonv1alpha1.ConfigSpecHash{
				ConfigReferent: addonv1alpha1.ConfigReferent{
					Name:      "test-config",
					Namespace: "test-ns",
				},
				SpecHash: "xyz789",
			},
		},
	}

	ue = event.UpdateEvent{
		ObjectOld: oldAddon,
		ObjectNew: newAddon,
	}
	if !pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered when MCA status.configReferences changed")
	}

	// Test: Status specHash change should trigger reconcile
	newerAddon := newAddon.DeepCopy()
	newerAddon.Status.ConfigReferences[0].DesiredConfig.SpecHash = "abc123-updated"
	ue = event.UpdateEvent{
		ObjectOld: newAddon,
		ObjectNew: newerAddon,
	}
	if !pred.UpdateFunc(ue) {
		t.Fatal("reconcile not triggered when MCA status specHash changed")
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
			ResourceVersion: "1",
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
			ResourceVersion: "2",
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

	newWork.ObjectMeta.ResourceVersion = "1"
	ue = event.UpdateEvent{
		ObjectNew: newWork,
		ObjectOld: work,
	}
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile triggered for managedclusteraddon update event when resourceVersion not updated")
	}
}

func TestGetMchPred(t *testing.T) {
	c := fake.NewClientBuilder().WithRuntimeObjects().Build()
	pred := getMchPred(c)
	mch := &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "mch",
			Namespace:       config.GetMCONamespace(),
			ResourceVersion: "1",
		},
		Status: mchv1.MultiClusterHubStatus{
			CurrentVersion: "1.0",
			DesiredVersion: "1.0",
		},
	}
	ce := event.CreateEvent{
		Object: mch,
	}
	if pred.CreateFunc(ce) {
		t.Fatal("reconcile triggered for mch create event")
	}
	ce.Object.SetNamespace("update_ns")
	if pred.CreateFunc(ce) {
		t.Fatal("reconcile triggered for mch create event")
	}

	de := event.DeleteEvent{
		Object: mch,
	}
	if pred.DeleteFunc(de) {
		t.Fatal("reconcile triggered for mch delete event")
	}

	oldMch := &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "mch",
			Namespace:       config.GetMCONamespace(),
			ResourceVersion: "0",
		},
	}
	ue := event.UpdateEvent{
		ObjectNew: mch,
		ObjectOld: oldMch,
	}
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile triggered for mch update event")
	}
	ue.ObjectNew.SetNamespace("update_ns")
	if pred.UpdateFunc(ue) {
		t.Fatal("reconcile triggered for mch update event")
	}
}
