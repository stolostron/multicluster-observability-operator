// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package openshift

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	// "github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/status"
	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/status"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type statusUpdaterI interface {
	UpdateComponentCondition(context.Context, status.Component, status.Reason, string) (bool, error)
	GetConditionReason(context.Context, status.Component) (status.Reason, error)
}

type CmoConfigChangesWatcher struct {
	statusReporter       statusUpdaterI
	leakyBucket          *leakyBucket
	logger               logr.Logger
	checkedAtLeastOnce   bool // Ensures the status is processed and reset on restarts to avoid locked state
	statusResetFillRatio float64
}

// NewCmoConfigChangesWatcher creates a monitoring component that detects excessive CMO configMap driven reconciliations
// using a leaky bucket algorithm. This helps detecting potential reconciliation conflicts between operators.
// When such issue is detected, it degrades the addon status with relevant message, making this issue visible
// to the user.
//
// Behavior:
// 1. Uses a leaky bucket to track excessive reconciliations while enabling bursts:
//   - Bucket fills on each CMO configMap triggered reconciliation
//   - Drains one item every bucketLeakPeriod
//
// 2. Degrades system status when the bucket reaches capacity
// 3. Restores normal status when the bucket fill ratio drops below statusResetFillRatio
func NewCmoConfigChangesWatcher(
	_ client.Client,
	logger logr.Logger,
	statusReporter statusUpdaterI,
	bucketCapacity int,
	bucketLeakPeriod time.Duration,
	statusResetFillRatio float64,
) *CmoConfigChangesWatcher {
	return &CmoConfigChangesWatcher{
		leakyBucket:          newLeakyBucket(bucketCapacity, bucketLeakPeriod),
		statusReporter:       statusReporter,
		logger:               logger,
		statusResetFillRatio: statusResetFillRatio,
	}
}

// CheckRequest assesses whether CMO configMap changes are triggering excessive reconciliations and updates system status accordingly.
// When the status becomes degraded, it returns optimal requeue timing for status recovery checks.
func (c *CmoConfigChangesWatcher) CheckRequest(ctx context.Context, req ctrl.Request, cmoWasUpdated bool) (ctrl.Result, error) {
	if c.checkedAtLeastOnce && (req.Namespace != config.OCPClusterMonitoringNamespace || req.Name != config.OCPClusterMonitoringConfigMapName) {
		return ctrl.Result{}, nil
	}

	c.checkedAtLeastOnce = true

	if cmoWasUpdated {
		// Only add to the bucket on effective cmo configuration updates. Otherwise, requeues for status update will continue filling the bucket.
		if ok := c.leakyBucket.Add(); !ok {
			// Is already full. Degrade status and requeue after estimated delay
			c.logger.Info(
				"Detected excessive reconciliations triggered by CMO configurations, potentially resulting from reconciliation conflicts between operators. Degrading the addon status.",
				"request",
				req.String(),
			)
			wasReported, err := c.statusReporter.UpdateComponentCondition(ctx, status.MetricsCollector, status.CmoReconcileLoopDetected, "CMO configuration is being constantly updated.")
			if err != nil {
				if errors.Is(err, status.ErrInvalidTransition) {
					c.logger.Info("Addon status is in an incompatible state to transition, ignoring invalid transition", "message", err.Error())
				} else {
					c.logger.Error(err, "Failed to report status")
				}
			}
			if wasReported {
				c.logger.Info("Status updated", "component", status.MetricsCollector, "reason", status.CmoReconcileLoopDetected)
			}

			// Requeue after the bucket has reached statusResetFillRatio
			requeueAfter := c.leakyBucket.LeakPeriod * time.Duration(math.Ceil(float64(c.leakyBucket.Capacity)*(1-c.statusResetFillRatio)))
			c.logger.Info("Requeueing with delay to ensure status update", "requeueAfter", requeueAfter)
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}

	// Get out early if the addon status is not the one managed by this function
	currentReason, err := c.statusReporter.GetConditionReason(ctx, status.MetricsCollector)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get current addon status reason: %w", err)
	}

	if currentReason != status.CmoReconcileLoopDetected {
		return ctrl.Result{}, nil
	}

	// Only restore the addon state once the bucket has emptied anough to avoid flapping status
	if c.leakyBucket.FillRatio() <= c.statusResetFillRatio {
		// Bucket is emptying, reset the status
		c.logger.Info("Excessive reconciliations triggered by CMO configurations is over. Resetting the addon status into Progressing state.")
		if wasReported, err := c.statusReporter.UpdateComponentCondition(ctx, status.MetricsCollector, status.CmoReconcileLoopStopped, "CMO configuration updates have stopped."); err != nil {
			c.logger.Error(err, "Failed to report status")
		} else if wasReported {
			c.logger.Info("Status updated", "component", status.MetricsCollector, "reason", status.CmoReconcileLoopStopped)
		}
	} else {
		// Ensure this is requeued to avoid locked state.
		// Computed approximate remaining time to statusResetFillRatio for better system responsiveness.
		targetCapacity := int(float64(c.leakyBucket.Capacity) * c.statusResetFillRatio)
		remainingLen := max(c.leakyBucket.Len()-targetCapacity, 1)
		requeueAfter := c.leakyBucket.LeakPeriod * time.Duration(remainingLen)
		c.logger.Info(
			"Requeueing with delay to ensure status update, bucket fill ratio is too high to restore the addon status",
			"requeueAfter",
			requeueAfter,
			"bucketFillRatio",
			c.leakyBucket.FillRatio(),
		)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

type leakyBucket struct {
	LeakPeriod time.Duration
	Capacity   int
	bucket     chan struct{}
}

func newLeakyBucket(capacity int, leakPeriod time.Duration) *leakyBucket {
	ret := &leakyBucket{
		bucket:     make(chan struct{}, capacity),
		LeakPeriod: leakPeriod,
		Capacity:   capacity,
	}

	go ret.startLeaking()

	return ret
}

// Add returns false if the bucket is already full, true otherwise
func (l *leakyBucket) Add() bool {
	select {
	case l.bucket <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *leakyBucket) FillRatio() float64 {
	return float64(len(l.bucket)) / float64(cap(l.bucket))
}

func (l *leakyBucket) Len() int {
	return len(l.bucket)
}

func (l *leakyBucket) startLeaking() {
	ticker := time.NewTicker(l.LeakPeriod)
	for range ticker.C {
		select {
		case <-l.bucket:
		default:
		}
	}
}
