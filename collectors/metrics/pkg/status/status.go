// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/status"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	addonName      = "observability-addon"
	addonNamespace = "open-cluster-management-addon-observability"
)

type Reporter interface {
	UpdateStatus(ctx context.Context, reason status.Reason, message string) error
}

var (
	_ Reporter = &StatusReport{}
	_ Reporter = &NoopReporter{}
)

type StatusReport struct {
	statusClient   client.Client
	standalone     bool
	isUwl          bool
	statusReporter status.Status
	logger         *slog.Logger
}

func New(c client.Client, logger *slog.Logger, standalone, isUwl bool) (*StatusReport, error) {
	logger.Info("Creating status client", "standalone", standalone, "isUwl", isUwl)

	statusLogger := logr.FromSlogHandler(slog.New(slog.NewTextHandler(os.Stdout, nil)).With("component", "statusclient").Handler())
	return &StatusReport{
		statusClient:   c,
		standalone:     standalone,
		isUwl:          isUwl,
		statusReporter: status.NewStatus(c, addonName, addonNamespace, statusLogger),
		logger:         logger,
	}, nil
}

func (s *StatusReport) UpdateStatus(ctx context.Context, reason status.Reason, message string) error {
	// Standalone mode is set when running on the hub cluster
	// In this case, we do not need to update the status of the ObservabilityAddon
	if s.standalone {
		return nil
	}

	component := status.MetricsCollector
	if s.isUwl {
		component = status.UwlMetricsCollector
	}

	if wasReported, err := s.statusReporter.UpdateComponentCondition(ctx, component, reason, message); err != nil {
		return err
	} else if wasReported {
		s.logger.Info("Status updated", "component", component, "reason", reason, "message", message)
	}

	return nil
}

type NoopReporter struct{}

func (s *NoopReporter) UpdateStatus(_ context.Context, _ status.Reason, _ string) error {
	return nil
}
