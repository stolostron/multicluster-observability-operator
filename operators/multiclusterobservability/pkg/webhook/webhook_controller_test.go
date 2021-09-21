// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook

import (
	"context"
	"reflect"
	"testing"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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
			client := fake.NewSimpleClientset()
			if c.existingmwh != nil {
				_, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.TODO(), c.existingmwh, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("failed to create the mutating webhook configuration: %v", err)
				}
			}
			if c.existingvwh != nil {
				_, err := client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(context.TODO(), c.existingvwh, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("failed to create the validating webhook configuration: %v", err)
				}
			}
			wc := NewWebhookController(client, c.reconciledmwh, c.reconciledvwh)
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			go func() {
				wc.Start(ctx)
			}()
			time.Sleep(1 * time.Second)
			if c.reconciledmwh != nil {
				foundMwhc, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.TODO(), c.reconciledmwh.GetName(), metav1.GetOptions{})
				if err != nil {
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
				foundVwhc, err := client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(context.TODO(), c.reconciledvwh.GetName(), metav1.GetOptions{})
				if err != nil {
					t.Fatalf("failed to get the validating webhook configuration: %v", err)
				}
				if !(foundVwhc.Webhooks[0].Name == c.reconciledvwh.Webhooks[0].Name &&
					reflect.DeepEqual(foundVwhc.Webhooks[0].AdmissionReviewVersions, c.reconciledvwh.Webhooks[0].AdmissionReviewVersions) &&
					reflect.DeepEqual(foundVwhc.Webhooks[0].Rules, c.reconciledvwh.Webhooks[0].Rules) &&
					reflect.DeepEqual(foundVwhc.Webhooks[0].ClientConfig.Service, c.reconciledvwh.Webhooks[0].ClientConfig.Service)) {
					t.Errorf("Got differences between the found ValidatingWebhookConfiguration and reconciled ValidatingWebhookConfiguration:\nfound:%v\nreconciled:%v\n", foundVwhc, c.reconciledmwh)
				}
			}
			// send the done signal to the webhook controller
			time.Sleep(4 * time.Second)

			if c.reconciledmwh != nil {
				_, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.TODO(), c.reconciledmwh.GetName(), metav1.GetOptions{})
				if !apierrors.IsNotFound(err) {
					t.Errorf("expect got 'not found error' for the reconciled MutatingWebhookConfiguration, but got error: %v", err)
				}
			}
			if c.reconciledvwh != nil {
				_, err := client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(context.TODO(), c.reconciledvwh.GetName(), metav1.GetOptions{})
				if !apierrors.IsNotFound(err) {
					t.Errorf("expect got 'not found error' for the reconciled ValidatingWebhookConfiguration, but got error: %v", err)
				}
			}
		})
	}
}
