// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook

import (
	"context"
	"reflect"
	"testing"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestWebhookController(t *testing.T) {
	testValidatingWebhookPath := "/validate-testing"
	testMutatingWebhookPath := "/mutate-testing"
	noSideEffects := admissionregistrationv1.SideEffectClassNone
	allScopeType := admissionregistrationv1.AllScopes
	webhookServicePort := int32(443)
	testmwh := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testingmwh",
			Labels: map[string]string{
				"name": "testingmwh",
			},
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
				Name:                    "testingmwhook",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Name:      "testing-webhook-service",
						Namespace: "testing-webhook-service-namespace",
						Path:      &testValidatingWebhookPath,
						Port:      &webhookServicePort,
					},
					CABundle: []byte(""),
				},
				SideEffects: &noSideEffects,
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Create,
							admissionregistrationv1.Update,
						},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"observability.open-cluster-management.io"},
							APIVersions: []string{"v1beta2"},
							Resources:   []string{"multiclusterobservabilities"},
							Scope:       &allScopeType,
						},
					},
				},
			},
		},
	}
	testvwh := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testingvwh",
			Labels: map[string]string{
				"name": "testingvwh",
			},
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
				Name:                    "testingvwhook",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Name:      "testing-webhook-service",
						Namespace: "testing-webhook-service-namespace",
						Path:      &testMutatingWebhookPath,
						Port:      &webhookServicePort,
					},
					CABundle: []byte(""),
				},
				SideEffects: &noSideEffects,
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Create,
							admissionregistrationv1.Update,
						},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"observability.open-cluster-management.io"},
							APIVersions: []string{"v1beta2"},
							Resources:   []string{"multiclusterobservabilities"},
							Scope:       &allScopeType,
						},
					},
				},
			},
		},
	}
	cases := []struct {
		name          string
		existingmwh   *admissionregistrationv1.MutatingWebhookConfiguration
		existingvwh   *admissionregistrationv1.ValidatingWebhookConfiguration
		reconciledmwh *admissionregistrationv1.MutatingWebhookConfiguration
		reconciledvwh *admissionregistrationv1.ValidatingWebhookConfiguration
	}{
		// {
		// 	"no existing and reconciled webhook configurations",
		// 	nil,
		// 	nil,
		// 	nil,
		// 	nil,
		// },
		{
			"no existing webhook configurations and create the reconciled webhook configurations",
			nil,
			nil,
			testmwh,
			testvwh,
		},
		{
			"existing webhook configurations and create the reconciled webhook configurations",
			testmwh,
			testvwh,
			testmwh,
			testvwh,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			objs := []runtime.Object{}
			if c.existingmwh != nil {
				objs = append(objs, c.existingmwh)
			}
			if c.existingvwh != nil {
				objs = append(objs, c.existingvwh)
			}
			cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
			wc := NewWebhookController(cl, c.reconciledmwh, c.reconciledvwh)
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				wc.Start(ctx)
			}()
			time.Sleep(1 * time.Second)
			cancel()

			if c.reconciledmwh != nil {
				foundMwhc := &admissionregistrationv1.MutatingWebhookConfiguration{}
				if err := cl.Get(context.TODO(), types.NamespacedName{Name: c.reconciledmwh.GetName()}, foundMwhc); err != nil {
					t.Fatalf("failed to get the mutating webhook configuration: %v", err)
				}
				if !(foundMwhc.Webhooks[0].Name == c.reconciledmwh.Webhooks[0].Name &&
					reflect.DeepEqual(foundMwhc.Webhooks[0].AdmissionReviewVersions, c.reconciledmwh.Webhooks[0].AdmissionReviewVersions) &&
					reflect.DeepEqual(foundMwhc.Webhooks[0].Rules, c.reconciledmwh.Webhooks[0].Rules) &&
					reflect.DeepEqual(foundMwhc.Webhooks[0].ClientConfig.Service, c.reconciledmwh.Webhooks[0].ClientConfig.Service)) {
					t.Errorf("Got differences between the found MutatingWebhookConfiguration and reconciled MutatingWebhookConfiguration:\nfound:%v\nreconciled:%v\n", foundMwhc, c.reconciledmwh)
				}
			}

			if c.reconciledvwh != nil {
				foundVwhc := &admissionregistrationv1.ValidatingWebhookConfiguration{}
				if err := cl.Get(context.TODO(), types.NamespacedName{Name: c.reconciledvwh.GetName()}, foundVwhc); err != nil {
					t.Fatalf("failed to get the validating webhook configuration: %v", err)
				}
				if !(foundVwhc.Webhooks[0].Name == c.reconciledvwh.Webhooks[0].Name &&
					reflect.DeepEqual(foundVwhc.Webhooks[0].AdmissionReviewVersions, c.reconciledvwh.Webhooks[0].AdmissionReviewVersions) &&
					reflect.DeepEqual(foundVwhc.Webhooks[0].Rules, c.reconciledvwh.Webhooks[0].Rules) &&
					reflect.DeepEqual(foundVwhc.Webhooks[0].ClientConfig.Service, c.reconciledvwh.Webhooks[0].ClientConfig.Service)) {
					t.Errorf("Got differences between the found ValidatingWebhookConfiguration and reconciled ValidatingWebhookConfiguration:\nfound:%v\nreconciled:%v\n", foundVwhc, c.reconciledvwh)
				}
			}
		})
	}
}
