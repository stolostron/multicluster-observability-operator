// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook

import (
	"context"
	"reflect"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("webhook-controller")

// WebhookController define the controller that manages(create, update and delete) the webhook configurations.
type WebhookController struct {
	client            kubernetes.Interface
	mutatingWebhook   *admissionregistrationv1.MutatingWebhookConfiguration
	validatingWebhook *admissionregistrationv1.ValidatingWebhookConfiguration
}

// NewWebhookController create the WebhookController.
func NewWebhookController(client kubernetes.Interface, mwh *admissionregistrationv1.MutatingWebhookConfiguration, vwh *admissionregistrationv1.ValidatingWebhookConfiguration) *WebhookController {
	return &WebhookController{
		client:            client,
		mutatingWebhook:   mwh,
		validatingWebhook: vwh,
	}
}

// Start runs the WebhookController with the given context.
// it will create the corresponding webhook configuration with the client
// at starting and remove it when context done singal is received
// currently the controller will not watch the change of the webhook configurations.
func (wc *WebhookController) Start(ctx context.Context) error {
	if wc.mutatingWebhook != nil {
		foundMwhc, err := wc.client.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.TODO(), wc.mutatingWebhook.GetName(), metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			_, err := wc.client.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.TODO(), wc.mutatingWebhook, metav1.CreateOptions{})
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			// there is an existing mutatingWebhookConfiguration
			if !(foundMwhc.Webhooks[0].Name == wc.mutatingWebhook.Webhooks[0].Name &&
				reflect.DeepEqual(foundMwhc.Webhooks[0].AdmissionReviewVersions, wc.mutatingWebhook.Webhooks[0].AdmissionReviewVersions) &&
				reflect.DeepEqual(foundMwhc.Webhooks[0].Rules, wc.mutatingWebhook.Webhooks[0].Rules) &&
				reflect.DeepEqual(foundMwhc.Webhooks[0].ClientConfig.Service, wc.mutatingWebhook.Webhooks[0].ClientConfig.Service)) {
				wc.mutatingWebhook.ObjectMeta.ResourceVersion = foundMwhc.ObjectMeta.ResourceVersion
				_, err := wc.client.AdmissionregistrationV1().MutatingWebhookConfigurations().Update(context.TODO(), wc.mutatingWebhook, metav1.UpdateOptions{})
				if err != nil {
					return err
				}
			}
		}
	}

	if wc.validatingWebhook != nil {
		foundVwhc, err := wc.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(context.TODO(), wc.validatingWebhook.GetName(), metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			_, err := wc.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(context.TODO(), wc.validatingWebhook, metav1.CreateOptions{})
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			// there is an existing mutatingWebhookConfiguration
			if !(foundVwhc.Webhooks[0].Name == wc.validatingWebhook.Webhooks[0].Name &&
				reflect.DeepEqual(foundVwhc.Webhooks[0].AdmissionReviewVersions, wc.validatingWebhook.Webhooks[0].AdmissionReviewVersions) &&
				reflect.DeepEqual(foundVwhc.Webhooks[0].Rules, wc.validatingWebhook.Webhooks[0].Rules) &&
				reflect.DeepEqual(foundVwhc.Webhooks[0].ClientConfig.Service, wc.validatingWebhook.Webhooks[0].ClientConfig.Service)) {
				wc.validatingWebhook.ObjectMeta.ResourceVersion = foundVwhc.ObjectMeta.ResourceVersion
				_, err := wc.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(context.TODO(), wc.validatingWebhook, metav1.UpdateOptions{})
				if err != nil {
					return err
				}
			}
		}
	}

	// wait for context done signal
	<-ctx.Done()

	if wc.mutatingWebhook != nil {
		// delete the mutatingwebhookconfiguration and ignore error
		if err := wc.client.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(context.TODO(), wc.mutatingWebhook.GetName(), metav1.DeleteOptions{}); err != nil {
			log.V(1).Info("error to delete the mutatingwebhookconfiguration", "mutatingwebhookconfiguration", wc.mutatingWebhook.GetName())
		}
	}
	if wc.validatingWebhook != nil {
		// delete the validatingwebhookconfiguration and ignore error
		if err := wc.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(context.TODO(), wc.validatingWebhook.GetName(), metav1.DeleteOptions{}); err != nil {
			log.V(1).Info("error to delete the validatingwebhookconfiguration", "validatingwebhookconfiguration", wc.validatingWebhook.GetName(), "message", err)
		}
	}

	return nil
}
