package placementrule

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const (
	testNamespace = "test-ns"
)

func int32Ptr(i int32) *int32 { return &i }

func TestClusterPred(t *testing.T) {
	name := "test-obj"
	caseList := []struct {
		caseName       string
		namespace      string
		annotations    map[string]string
		expectedCreate bool
		expectedUpdate bool
		expectedDelete bool
	}{
		{
			caseName:       "Disable Automatic Install",
			namespace:      testNamespace,
			annotations:    map[string]string{disableAddonAutomaticInstallationAnnotationKey: "true"},
			expectedCreate: false,
			expectedUpdate: false,
			expectedDelete: false,
		},
		{
			caseName:       "Automatic Install",
			namespace:      testNamespace,
			annotations:    nil,
			expectedCreate: true,
			expectedUpdate: true,
			expectedDelete: true,
		},
	}

	for _, c := range caseList {
		t.Run(c.caseName, func(t *testing.T) {
			pred := getClusterPreds()
			create_event := event.CreateEvent{
				Object: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:        name,
						Namespace:   c.namespace,
						Annotations: c.annotations,
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: int32Ptr(2),
					},
				},
			}

			if c.expectedCreate {
				if !pred.CreateFunc(create_event) {
					t.Fatalf("pre func return false on applied createevent in case: (%v)", c.caseName)
				}
				create_event.Object.SetName(name + "test")
				if pred.CreateFunc(create_event) {
					t.Fatalf("pre func return true on different obj name in case: (%v)", c.caseName)
				}
			} else {
				if pred.CreateFunc(create_event) {
					t.Fatalf("pre func return true on non-applied createevent in case: (%v)", c.caseName)
				}
			}

			update_event := event.UpdateEvent{
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
				if !pred.UpdateFunc(update_event) {
					t.Fatalf("pre func return false on applied update event in case: (%v)", c.caseName)
				}
				update_event.ObjectNew.SetResourceVersion("1")
				if pred.UpdateFunc(update_event) {
					t.Fatalf("pre func return true on same resource version in case: (%v)", c.caseName)
				}
				update_event.ObjectNew.SetResourceVersion("2")
				update_event.ObjectNew.(*appsv1.Deployment).Spec.Template.Spec.ServiceAccountName = "sa2"
				if pred.UpdateFunc(update_event) {
					t.Fatalf("pre func return true on same deployment spec in case: (%v)", c.caseName)
				}
			} else {
				if pred.UpdateFunc(update_event) {
					t.Fatalf("pre func return true on non-applied updateevent in case: (%v)", c.caseName)
				}
			}

			delete_event := event.DeleteEvent{
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
				if !pred.DeleteFunc(delete_event) {
					t.Fatalf("pre func return false on applied deleteevent in case: (%v)", c.caseName)
				}
				delete_event.Object.SetName(name + "test")
				if pred.DeleteFunc(delete_event) {
					t.Fatalf("pre func return true on different obj name in case: (%v)", c.caseName)
				}
			} else {
				if pred.DeleteFunc(delete_event) {
					t.Fatalf("HubInpre funcfoPred return true on deleteevent in case: (%v)", c.caseName)
				}
			}

		})
	}
}
