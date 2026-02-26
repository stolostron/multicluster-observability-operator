// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package webhook

import (
	"context"
	"reflect"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("webhook-controller")

// WebhookController define the controller that manages(create, update and delete) the webhook configurations.
// it implements the Runnable interface from https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/manager#Runnable
type WebhookController struct {
	client            client.Client
	mutatingWebhook   *admissionregistrationv1.MutatingWebhookConfiguration
	validatingWebhook *admissionregistrationv1.ValidatingWebhookConfiguration
}

// NewWebhookController create the WebhookController.
func NewWebhookController(
	client client.Client,
	mwh *admissionregistrationv1.MutatingWebhookConfiguration,
	vwh *admissionregistrationv1.ValidatingWebhookConfiguration,
) *WebhookController {
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
		log.V(1).Info(
			"creating or updating the mutatingwebhookconfiguration",
			"mutatingwebhookconfiguration",
			wc.mutatingWebhook.GetName())
		foundMwhc := &admissionregistrationv1.MutatingWebhookConfiguration{}
		err := wc.client.Get(
			ctx,
			types.NamespacedName{Name: wc.mutatingWebhook.GetName()}, foundMwhc)
		switch {
		case err != nil && apierrors.IsNotFound(err):
			if err := wc.client.Create(ctx, wc.mutatingWebhook); err != nil {
				log.V(1).Info("failed to create the mutatingwebhookconfiguration",
					"mutatingwebhookconfiguration", wc.mutatingWebhook.GetName(),
					"error", err)
				return err
			}
			log.V(1).Info("the mutatingwebhookconfiguration is created",
				"mutatingwebhookconfiguration", wc.mutatingWebhook.GetName())
		case err != nil:
			log.V(1).Info("failed to check the mutatingwebhookconfiguration", "mutatingwebhookconfiguration", wc.mutatingWebhook.GetName(), "error", err)
			return err
		default:
			// there is an existing mutatingWebhookConfiguration
			if len(foundMwhc.Webhooks) != len(wc.mutatingWebhook.Webhooks) ||
				(foundMwhc.Webhooks[0].Name != wc.mutatingWebhook.Webhooks[0].Name ||
					!reflect.DeepEqual(foundMwhc.Webhooks[0].AdmissionReviewVersions, wc.mutatingWebhook.Webhooks[0].AdmissionReviewVersions) ||
					!reflect.DeepEqual(foundMwhc.Webhooks[0].Rules, wc.mutatingWebhook.Webhooks[0].Rules) ||
					!reflect.DeepEqual(foundMwhc.Webhooks[0].ClientConfig.Service, wc.mutatingWebhook.Webhooks[0].ClientConfig.Service)) {
				wc.mutatingWebhook.ResourceVersion = foundMwhc.ResourceVersion
				if err := wc.client.Update(ctx, wc.mutatingWebhook); err != nil {
					log.V(1).Info("failed to update the mutatingwebhookconfiguration", "mutatingwebhookconfiguration", wc.mutatingWebhook.GetName(), "error", err)
					return err
				}
				log.V(1).Info("the mutatingwebhookconfiguration is updated", "mutatingwebhookconfiguration", wc.mutatingWebhook.GetName())
			}
		}
	}

	if wc.validatingWebhook != nil {
		log.V(1).Info("creating or updating the validatingwebhookconfiguration",
			"validatingwebhookconfiguration", wc.validatingWebhook.GetName())
		foundVwhc := &admissionregistrationv1.ValidatingWebhookConfiguration{}
		if err := wc.client.Get(ctx, types.NamespacedName{Name: wc.validatingWebhook.GetName()}, foundVwhc); err != nil &&
			apierrors.IsNotFound(err) {
			if err := wc.client.Create(ctx, wc.validatingWebhook); err != nil {
				log.V(1).Info("failed to create the validatingwebhookconfiguration",
					"validatingwebhookconfiguration", wc.validatingWebhook.GetName(),
					"error", err)
				return err
			}
			log.V(1).Info("the validatingwebhookconfiguration is created",
				"validatingwebhookconfiguration", wc.validatingWebhook.GetName())
		} else if err != nil {
			log.V(1).Info("failed to check the validatingwebhookconfiguration", "validatingwebhookconfiguration", wc.validatingWebhook.GetName(), "error", err)
			return err
		} else {
			// there is an existing validatingwebhookconfiguration
			if len(foundVwhc.Webhooks) != len(wc.validatingWebhook.Webhooks) ||
				(foundVwhc.Webhooks[0].Name != wc.validatingWebhook.Webhooks[0].Name ||
					!reflect.DeepEqual(foundVwhc.Webhooks[0].AdmissionReviewVersions, wc.validatingWebhook.Webhooks[0].AdmissionReviewVersions) ||
					!reflect.DeepEqual(foundVwhc.Webhooks[0].Rules, wc.validatingWebhook.Webhooks[0].Rules) ||
					!reflect.DeepEqual(foundVwhc.Webhooks[0].ClientConfig.Service, wc.validatingWebhook.Webhooks[0].ClientConfig.Service)) {
				wc.validatingWebhook.ResourceVersion = foundVwhc.ResourceVersion

				err := wc.client.Update(ctx, wc.validatingWebhook)
				if err != nil {
					log.V(1).Info("failed to update the validatingwebhookconfiguration", "validatingwebhookconfiguration", wc.validatingWebhook.GetName(), "error", err)
					return err
				}
				log.V(1).Info("the validatingwebhookconfiguration is updated", "validatingwebhookconfiguration", wc.validatingWebhook.GetName())
			}
			log.V(1).Info("the validatingwebhookconfiguration already exists and no change", "validatingwebhookconfiguration", wc.validatingWebhook.GetName())
		}
	}

	// wait for context done signal
	<-ctx.Done()

	// currently kubernetes prevents terminating pod from deleting kubernetes resources(including
	// validatingwebhookconfiguration...), see:
	// https://kubernetes.io/blog/2021/05/14/using-finalizers-to-control-deletion/
	// that's why the deleting webhook configuration code is commented
	/*
		log.V(1).Info("Shutdown signal received, waiting for the webhook cleanup.")
		if wc.mutatingWebhook != nil {
			// delete the mutatingwebhookconfiguration and ignore error
			err := wc.client.Delete(ctx, wc.mutatingWebhook, &client.DeleteOptions{})
			if err != nil {
				log.V(1).Info("error to delete the mutatingwebhookconfiguration", "mutatingwebhookconfiguration", wc.mutatingWebhook.GetName(), "error", err)
			}
		}
		if wc.validatingWebhook != nil {
			// delete the validatingwebhookconfiguration and ignore error
			err := wc.client.Delete(ctx, wc.validatingWebhook, &client.DeleteOptions{})
			if err != nil {
				log.V(1).Info("error to delete the validatingwebhookconfiguration", "validatingwebhookconfiguration", wc.validatingWebhook.GetName(), "error", err)
			}
		}
		log.V(1).Info("webhook clean up")
	*/

	return nil
}
