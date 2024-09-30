// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"testing"
	"time"

	clusterv1 "open-cluster-management.io/api/cluster/v1"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const (
	testNamespace = "test-ns"
)

func int32Ptr(i int32) *int32            { return &i }
func timePtr(t metav1.Time) *metav1.Time { return &t }

func TestClusterPred(t *testing.T) {
	name := "test-obj"
	caseList := []struct {
		caseName          string
		namespace         string
		annotations       map[string]string
		deletionTimestamp *metav1.Time
		expectedCreate    bool
		expectedUpdate    bool
		expectedDelete    bool
	}{
		{
			caseName:          "Disable Automatic Install",
			namespace:         testNamespace,
			annotations:       map[string]string{disableAddonAutomaticInstallationAnnotationKey: "true"},
			deletionTimestamp: nil,
			expectedCreate:    false,
			expectedUpdate:    false,
			expectedDelete:    false,
		},
		{
			caseName:          "Automatic Install",
			namespace:         testNamespace,
			annotations:       nil,
			deletionTimestamp: nil,
			expectedCreate:    true,
			expectedUpdate:    true,
			expectedDelete:    true,
		},
		{
			caseName:          "Deletion Timestamp",
			namespace:         testNamespace,
			annotations:       nil,
			deletionTimestamp: timePtr(metav1.NewTime(time.Now().Local().Add(time.Second * time.Duration(5)))),
			expectedCreate:    true,
			expectedUpdate:    true,
			expectedDelete:    true,
		},
	}

	for _, c := range caseList {
		t.Run(c.caseName, func(t *testing.T) {
			pred := getClusterPreds()
			create_event := event.CreateEvent{
				Object: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:            name,
						Namespace:       c.namespace,
						Annotations:     c.annotations,
						Labels:          map[string]string{"vendor": "OpenShift", "openshiftVersion": "4.6.0"},
						ResourceVersion: "1",
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			}

			if c.expectedCreate {
				if !pred.CreateFunc(create_event) {
					t.Fatalf("pre func return false on applied createevent in case: (%v)", c.caseName)
				}
			} else {
				if pred.CreateFunc(create_event) {
					t.Fatalf("pre func return true on non-applied createevent in case: (%v)", c.caseName)
				}
			}

			update_event := event.UpdateEvent{
				ObjectNew: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              name,
						Namespace:         c.namespace,
						ResourceVersion:   "2",
						DeletionTimestamp: c.deletionTimestamp,
						Annotations:       c.annotations,
						Labels:            map[string]string{"vendor": "OpenShift", "openshiftVersion": "4.6.0"},
					},
				},
				ObjectOld: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:            name,
						Namespace:       c.namespace,
						ResourceVersion: "1",
					},
				},
			}

			if c.expectedUpdate {
				if !pred.UpdateFunc(update_event) {
					t.Fatalf("pre func return false on applied update event in case: (%v)", c.caseName)
				}
				update_event.ObjectNew.SetResourceVersion("1")
				if pred.UpdateFunc(update_event) {
					t.Fatalf("pre func return true on same resource version in case: (%v)", c.caseName)
				}
				update_event.ObjectNew.SetResourceVersion("2")
			} else {
				if pred.UpdateFunc(update_event) {
					t.Fatalf("pre func return true on non-applied updateevent in case: (%v)", c.caseName)
				}
			}

			delete_event := event.DeleteEvent{
				Object: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:        name,
						Namespace:   c.namespace,
						Annotations: c.annotations,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			}

			if c.expectedDelete {
				if !pred.DeleteFunc(delete_event) {
					t.Fatalf("pre func return false on applied deleteevent in case: (%v)", c.caseName)
				}
			} else {
				if pred.DeleteFunc(delete_event) {
					t.Fatalf("HubInpre funcfoPred return true on deleteevent in case: (%v)", c.caseName)
				}
			}
		})
	}
}

func TestManagedClusterLabelReady(t *testing.T) {
	name := "test-obj"
	caseList := []struct {
		caseName          string
		namespace         string
		annotations       map[string]string
		deletionTimestamp *metav1.Time
		expectedCreate    bool
		expectedUpdate    bool
		expectedDelete    bool
		labels            map[string]string
	}{
		{
			caseName:       "ManagedCluster with vendor label autodetect",
			namespace:      testNamespace,
			expectedCreate: false,
			expectedUpdate: false,
			labels:         map[string]string{"vendor": "auto-detect"},
		},
		{
			caseName:       "ManagedCluster with vendor label openshift",
			namespace:      testNamespace,
			expectedUpdate: true,
			expectedCreate: true,
			labels:         map[string]string{"vendor": "OpenShift", "openshiftVersion": "4.6.0"},
		},
		{
			caseName:       "ManagedCluster with vendor label and no openshiftVersion",
			namespace:      testNamespace,
			expectedUpdate: false,
			expectedCreate: false,
			labels:         map[string]string{"vendor": "OpenShift"},
		},
		{
			caseName:       "ManagedCluster with vendor label AKS",
			namespace:      testNamespace,
			expectedUpdate: true,
			expectedCreate: true,
			labels:         map[string]string{"vendor": "Azure", "azureVersion": "1.19.6"},
		},
	}

	for _, c := range caseList {
		t.Run(c.caseName, func(t *testing.T) {
			pred := getClusterPreds()
			create_event := event.CreateEvent{
				Object: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:        name,
						Namespace:   c.namespace,
						Annotations: c.annotations,
						Labels:      c.labels,
					},
					Spec: clusterv1.ManagedClusterSpec{
						HubAcceptsClient: true,
					},
				},
			}
			if c.expectedCreate {
				if !pred.CreateFunc(create_event) {
					t.Fatalf("pre func return false on applied createevent in case: (%v)", c.caseName)
				}
			} else {
				if pred.CreateFunc(create_event) {
					t.Fatalf("pre func return true on non-applied createevent in case: (%v)", c.caseName)
				}
			}

			update_event := event.UpdateEvent{
				ObjectNew: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              name,
						Namespace:         c.namespace,
						ResourceVersion:   "2",
						DeletionTimestamp: c.deletionTimestamp,
						Annotations:       c.annotations,
						Labels:            c.labels,
					},
				},

				ObjectOld: &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:            name,
						Namespace:       c.namespace,
						ResourceVersion: "1",
					},
				},
			}
			if c.expectedUpdate {
				if !pred.UpdateFunc(update_event) {
					t.Fatalf("pre func return false on applied update event in case: (%v)", c.caseName)
				}
			} else {
				if pred.UpdateFunc(update_event) {
					t.Fatalf("pre func return true on non-applied updateevent in case: (%v)", c.caseName)
				}
			}
		})
	}
}

func TestAddOnDeploymentConfigPredicate(t *testing.T) {
	name := "test-obj"
	caseList := []struct {
		caseName       string
		namespace      string
		expectedCreate bool
		expectedUpdate bool
		expectedDelete bool
		createEvent    *event.CreateEvent
		updateEvent    func() event.UpdateEvent
		deleteEvent    *event.DeleteEvent
	}{
		{
			caseName:       "Create AddonDeploymentConfig",
			namespace:      testNamespace,
			expectedCreate: true,
			createEvent: &event.CreateEvent{
				Object: defaultAddonDeploymentConfig,
			},
		},
		{
			caseName:       "Update AddonDeploymentConfig with Spec.ProxyConfig changes",
			namespace:      testNamespace,
			expectedUpdate: true,
			updateEvent: func() event.UpdateEvent {
				newDefaultAddonDeploymentConfig := defaultAddonDeploymentConfig.DeepCopy()
				newDefaultAddonDeploymentConfig.Spec.ProxyConfig.HTTPProxy = "http://bar1.com"
				return event.UpdateEvent{
					ObjectOld: defaultAddonDeploymentConfig,
					ObjectNew: newDefaultAddonDeploymentConfig,
				}
			},
		},
		{
			caseName:       "Update AddonDeploymentConfig with Spec.NodePlacement changes",
			namespace:      testNamespace,
			expectedUpdate: true,
			updateEvent: func() event.UpdateEvent {
				newDefaultAddonDeploymentConfig := defaultAddonDeploymentConfig.DeepCopy()
				newDefaultAddonDeploymentConfig.Spec.NodePlacement = &addonv1alpha1.NodePlacement{
					NodeSelector: map[string]string{"foo": "bar"},
				}
				return event.UpdateEvent{
					ObjectOld: defaultAddonDeploymentConfig,
					ObjectNew: newDefaultAddonDeploymentConfig,
				}
			},
		},
		{
			caseName:       "Update AddonDeploymentConfig without Spec changes",
			namespace:      testNamespace,
			expectedUpdate: false,
			updateEvent: func() event.UpdateEvent {
				newDefaultAddonDeploymentConfig := defaultAddonDeploymentConfig.DeepCopy()
				newDefaultAddonDeploymentConfig.Labels = map[string]string{"foo": "bar"}
				return event.UpdateEvent{
					ObjectOld: defaultAddonDeploymentConfig,
					ObjectNew: newDefaultAddonDeploymentConfig,
				}
			},
		},
		{
			caseName:       "Update AddonDeploymentConfig with the same Spec ",
			namespace:      testNamespace,
			expectedUpdate: false,
			updateEvent: func() event.UpdateEvent {
				newDefaultAddonDeploymentConfig := defaultAddonDeploymentConfig.DeepCopy()
				return event.UpdateEvent{
					ObjectOld: defaultAddonDeploymentConfig,
					ObjectNew: newDefaultAddonDeploymentConfig,
				}
			},
		},
		{
			caseName:       "Delete AddonDeploymentConfig",
			namespace:      testNamespace,
			expectedDelete: true,
			deleteEvent: &event.DeleteEvent{
				Object: defaultAddonDeploymentConfig,
			},
		},
	}

	defaultAddonDeploymentConfig = &addonv1alpha1.AddOnDeploymentConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: addonv1alpha1.AddOnDeploymentConfigSpec{
			ProxyConfig: addonv1alpha1.ProxyConfig{
				HTTPProxy:  "http://foo.com",
				HTTPSProxy: "https://foo.com",
				NoProxy:    "bar.com",
			},
		},
	}
	for _, c := range caseList {
		t.Run(c.caseName, func(t *testing.T) {
			pred := GetAddOnDeploymentConfigPredicates()

			if c.createEvent != nil {
				gotCreate := pred.CreateFunc(*c.createEvent)
				if gotCreate != c.expectedCreate {
					t.Fatalf("%s: expected predicate to return '%v' on applied create event. Got '%v'", c.caseName, c.expectedCreate, gotCreate)
				}
			}

			if c.updateEvent != nil {
				gotUpdate := pred.UpdateFunc(c.updateEvent())
				if gotUpdate != c.expectedUpdate {
					t.Fatalf("%s: expected predicate to return '%v' on applied update event. Got '%v'", c.caseName, c.expectedUpdate, gotUpdate)
				}
			}

			if c.deleteEvent != nil {
				gotDelete := pred.DeleteFunc(*c.deleteEvent)
				if gotDelete != c.expectedDelete {
					t.Fatalf("%s: expected predicate to return '%v' on applied delete event. Got '%v'", c.caseName, c.expectedDelete, gotDelete)
				}
			}
		})
	}
}
