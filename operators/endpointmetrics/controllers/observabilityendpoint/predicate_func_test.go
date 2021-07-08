// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package observabilityendpoint

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestPredFunc(t *testing.T) {
	name := "test-obj"
	caseList := []struct {
		caseName       string
		namespace      string
		create         bool
		update         bool
		delete         bool
		expectedCreate bool
		expectedUpdate bool
		expectedDelete bool
	}{
		{
			caseName:       "All false",
			namespace:      testNamespace,
			create:         false,
			update:         false,
			delete:         false,
			expectedCreate: false,
			expectedUpdate: false,
			expectedDelete: false,
		},
		{
			caseName:       "All true",
			namespace:      testNamespace,
			create:         true,
			update:         true,
			delete:         true,
			expectedCreate: true,
			expectedUpdate: true,
			expectedDelete: true,
		},
		{
			caseName:       "All true for cluster scope obj",
			namespace:      "",
			create:         true,
			update:         true,
			delete:         true,
			expectedCreate: true,
			expectedUpdate: true,
			expectedDelete: true,
		},
	}

	for _, c := range caseList {
		t.Run(c.caseName, func(t *testing.T) {
			pred := getPred(name, c.namespace, c.create, c.update, c.delete)
			ce := event.CreateEvent{
				Object: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: c.namespace,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(2),
					},
				},
			}
			if c.expectedCreate {
				if !pred.CreateFunc(ce) {
					t.Fatalf("pre func return false on applied createevent in case: (%v)", c.caseName)
				}
				ce.Object.SetName(name + "test")
				if pred.CreateFunc(ce) {
					t.Fatalf("pre func return true on different obj name in case: (%v)", c.caseName)
				}
			} else {
				if pred.CreateFunc(ce) {
					t.Fatalf("pre func return true on non-applied createevent in case: (%v)", c.caseName)
				}
			}

			ue := event.UpdateEvent{
				ObjectNew: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:            name,
						Namespace:       c.namespace,
						ResourceVersion: "2",
					},
					Spec: appsv1.DeploymentSpec{
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								ServiceAccountName: "sa1",
							},
						},
					},
				},
				ObjectOld: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:            name,
						Namespace:       c.namespace,
						ResourceVersion: "1",
					},
					Spec: appsv1.DeploymentSpec{
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								ServiceAccountName: "sa2",
							},
						},
					},
				},
			}
			if c.expectedUpdate {
				if !pred.UpdateFunc(ue) {
					t.Fatalf("pre func return false on applied update event in case: (%v)", c.caseName)
				}
				ue.ObjectNew.SetResourceVersion("1")
				if pred.UpdateFunc(ue) {
					t.Fatalf("pre func return true on same resource version in case: (%v)", c.caseName)
				}
				ue.ObjectNew.SetResourceVersion("2")
				ue.ObjectNew.(*appsv1.Deployment).Spec.Template.Spec.ServiceAccountName = "sa2"
				if pred.UpdateFunc(ue) {
					t.Fatalf("pre func return true on same deployment spec in case: (%v)", c.caseName)
				}
			} else {
				if pred.UpdateFunc(ue) {
					t.Fatalf("pre func return true on non-applied updateevent in case: (%v)", c.caseName)
				}
			}

			de := event.DeleteEvent{
				Object: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: c.namespace,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(2),
					},
				},
			}
			if c.expectedDelete {
				if !pred.DeleteFunc(de) {
					t.Fatalf("pre func return false on applied deleteevent in case: (%v)", c.caseName)
				}
				de.Object.SetName(name + "test")
				if pred.DeleteFunc(de) {
					t.Fatalf("pre func return true on different obj name in case: (%v)", c.caseName)
				}
			} else {
				if pred.DeleteFunc(de) {
					t.Fatalf("HubInpre funcfoPred return true on deleteevent in case: (%v)", c.caseName)
				}
			}
		})
	}
}
