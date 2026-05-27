// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

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
	testNamespace := "test-ns"
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
			replicas := int32(2)
			pred := getPred(name, c.namespace, c.create, c.update, c.delete)
			ce := event.CreateEvent{
				Object: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: c.namespace,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
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
						Replicas: &replicas,
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

func TestConfigMapDataChangedPredicate(t *testing.T) {
	name := "test-cm"
	namespace := "test-ns"

	tests := []struct {
		name      string
		predName  string
		predNS    string
		oldCM     *v1.ConfigMap
		newCM     *v1.ConfigMap
		expUpdate bool
		expCreate bool
		expDelete bool
	}{
		{
			name:     "Wildcard - Data changed",
			predName: "",
			predNS:   "",
			oldCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Data:       map[string]string{"foo": "bar"},
			},
			newCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Data:       map[string]string{"foo": "baz"},
			},
			expUpdate: true,
			expCreate: true,
			expDelete: true,
		},
		{
			name:     "Wildcard - Data unchanged",
			predName: "",
			predNS:   "",
			oldCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Data:       map[string]string{"foo": "bar"},
			},
			newCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Data:       map[string]string{"foo": "bar"},
			},
			expUpdate: false,
			expCreate: true,
			expDelete: true,
		},
		{
			name:     "Specific match - Name mismatch",
			predName: "other-cm",
			predNS:   namespace,
			oldCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			},
			newCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Data:       map[string]string{"foo": "bar"},
			},
			expUpdate: false,
			expCreate: false,
			expDelete: false,
		},
		{
			name:     "Specific match - Namespace mismatch",
			predName: name,
			predNS:   "other-ns",
			oldCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			},
			newCM: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Data:       map[string]string{"foo": "bar"},
			},
			expUpdate: false,
			expCreate: false,
			expDelete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pred := ConfigMapDataChangedPredicate(tt.predName, tt.predNS)

			ce := event.CreateEvent{Object: tt.newCM}
			if pred.CreateFunc(ce) != tt.expCreate {
				t.Errorf("CreateFunc expected %v", tt.expCreate)
			}

			de := event.DeleteEvent{Object: tt.newCM}
			if pred.DeleteFunc(de) != tt.expDelete {
				t.Errorf("DeleteFunc expected %v", tt.expDelete)
			}

			ue := event.UpdateEvent{ObjectOld: tt.oldCM, ObjectNew: tt.newCM}
			if pred.UpdateFunc(ue) != tt.expUpdate {
				t.Errorf("UpdateFunc expected %v", tt.expUpdate)
			}
		})
	}
}

func TestSecretDataChangedPredicate(t *testing.T) {
	name := "test-secret"
	namespace := "test-ns"

	tests := []struct {
		name      string
		predName  string
		predNS    string
		oldSrt    *v1.Secret
		newSrt    *v1.Secret
		expUpdate bool
		expCreate bool
		expDelete bool
	}{
		{
			name:     "Wildcard - Data changed",
			predName: "",
			predNS:   "",
			oldSrt: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Data:       map[string][]byte{"token": []byte("old")},
			},
			newSrt: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Data:       map[string][]byte{"token": []byte("new")},
			},
			expUpdate: true,
			expCreate: true,
			expDelete: true,
		},
		{
			name:     "Specific match - Name match",
			predName: name,
			predNS:   namespace,
			oldSrt: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			},
			newSrt: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Data:       map[string][]byte{"token": []byte("new")},
			},
			expUpdate: true,
			expCreate: true,
			expDelete: true,
		},
		{
			name:      "Type mismatch",
			predName:  "",
			predNS:    "",
			oldSrt:    &v1.Secret{},
			newSrt:    nil, // This should trigger the !ok check
			expUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pred := SecretDataChangedPredicate(tt.predName, tt.predNS)

			if tt.newSrt != nil {
				ce := event.CreateEvent{Object: tt.newSrt}
				if pred.CreateFunc(ce) != tt.expCreate {
					t.Errorf("CreateFunc expected %v", tt.expCreate)
				}

				de := event.DeleteEvent{Object: tt.newSrt}
				if pred.DeleteFunc(de) != tt.expDelete {
					t.Errorf("DeleteFunc expected %v", tt.expDelete)
				}
			}

			ue := event.UpdateEvent{ObjectOld: tt.oldSrt, ObjectNew: tt.newSrt}
			if pred.UpdateFunc(ue) != tt.expUpdate {
				t.Errorf("UpdateFunc expected %v", tt.expUpdate)
			}
		})
	}
}
