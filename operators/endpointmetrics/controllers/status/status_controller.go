// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

type ClientWithReloader interface {
	client.Client
	Reload() error
}

// StatusReconciler reconciles status object.
type StatusReconciler struct {
	Client       client.Client
	HubNamespace string
	Namespace    string
	HubClient    ClientWithReloader
	ObsAddonName string
	Logger       logr.Logger
}

// Reconcile reads that state of the cluster for a ObservabilityAddon object and makes changes based on the state read
// and what is in the ObservabilityAddon.Status
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *StatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Logger.WithValues("Request", req.String())
	r.Logger.Info("Reconciling")

	// Fetch the ObservabilityAddon instance in hub cluster
	hubObsAddon := &oav1beta1.ObservabilityAddon{}
	err := r.HubClient.Get(ctx, types.NamespacedName{Name: r.ObsAddonName, Namespace: r.HubNamespace}, hubObsAddon)
	if err != nil {
		if isAuthOrConnectionErr(err) {
			// Try reloading the kubeconfig for the hub cluster
			if err := r.HubClient.Reload(); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to reload the hub client: %w", err)
			}
			r.Logger.Info("Failed to get ObservabilityAddon in hub cluster, reloaded hub, requeue with delay", "error", err)
			return ctrl.Result{Requeue: true}, nil
		}

		if isTransientErr(err) {
			r.Logger.Info("Failed to get ObservabilityAddon in hub cluster, requeue with delay", "error", err)
			return requeueWithOptionalDelay(err), nil
		}

		return ctrl.Result{}, err
	}

	// Retry on conflict as operation happens in other cluster
	// on a shared resource that can be updated by multiple controllers.
	retryErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Fetch the ObservabilityAddon instance in local cluster
		obsAddon := &oav1beta1.ObservabilityAddon{}
		if err != r.Client.Get(ctx, types.NamespacedName{Name: r.ObsAddonName, Namespace: r.Namespace}, obsAddon) {
			return err
		}

		// Only update the status in hub cluster if needed
		if reflect.DeepEqual(hubObsAddon.Status, obsAddon.Status) {
			return nil
		}

		updatedAddon := hubObsAddon.DeepCopy()
		updatedAddon.Status = obsAddon.Status

		// Update the status in hub cluster
		return r.HubClient.Status().Update(ctx, updatedAddon)
	})
	if retryErr != nil {
		if isTransientErr(retryErr) || errors.IsConflict(retryErr) {
			r.Logger.Info("Retryable error while updating status, request will be retried.", "error", retryErr)
			return requeueWithOptionalDelay(retryErr), nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to update status in hub cluster: %w", retryErr)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StatusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetNamespace() == r.Namespace &&
				!reflect.DeepEqual(e.ObjectNew.(*oav1beta1.ObservabilityAddon).Status,
					e.ObjectOld.(*oav1beta1.ObservabilityAddon).Status) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&oav1beta1.ObservabilityAddon{}, builder.WithPredicates(pred)).
		Complete(r)
}

// isTransientErr checks if the error is a transient error
// This suggests that a retry (with any change) might be successful
func isTransientErr(err error) bool {
	if _, ok := err.(net.Error); ok {
		return true
	}

	if statusErr, ok := err.(*errors.StatusError); ok {
		code := statusErr.Status().Code
		if code >= 500 && code < 600 && code != 501 {
			return true
		}
	}

	return errors.IsTimeout(err) || errors.IsServerTimeout(err) || errors.IsTooManyRequests(err)
}

// isAuthOrConnectionErr checks if the error is an authentication error or a connection error
// This suggests an issue with the client configuration and a reload might be needed
func isAuthOrConnectionErr(err error) bool {
	if errors.IsUnauthorized(err) || errors.IsForbidden(err) || errors.IsTimeout(err) {
		return true
	}

	if _, ok := err.(net.Error); ok {
		return true
	}

	return false
}

// requeueWithOptionalDelay requeues the request with a delay if suggested by the error
// Otherwise, it requeues the request without a delay
func requeueWithOptionalDelay(err error) ctrl.Result {
	if delay, ok := errors.SuggestsClientDelay(err); ok {
		return ctrl.Result{RequeueAfter: time.Duration(delay) * time.Second}
	}

	return ctrl.Result{Requeue: true}
}

// ClientGenerator is a function type that generates an instance of client.Client
type ClientGenerator func() (client.Client, error)

// ClientWithReload is a struct that implements the HubClientWithReload interface
// It uses a generator function to provide new instances of client.Client
type ClientWithReload struct {
	client.Client                 // Current client instance
	Generator     ClientGenerator // Function to generate a new client
}

// Reload creates a new client instance using the generator
func (c *ClientWithReload) Reload() error {
	var err error
	c.Client, err = c.Generator() // Generate and update the current client instance
	return err
}
