// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/go-kit/log"
	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/status"
)

const (
	addonName      = "observability-addon"
	addonNamespace = "open-cluster-management-addon-observability"
)

type StatusReport struct {
	statusClient   client.Client
	standalone     bool
	isUwl          bool
	statusReporter status.Status
	logger         log.Logger
}

func New(logger log.Logger, standalone, isUwl bool) (*StatusReport, error) {
	testMode := os.Getenv("UNIT_TEST") != ""
	var kubeClient client.Client
	if testMode {
		s := scheme.Scheme
		if err := oav1beta1.AddToScheme(s); err != nil {
			return nil, errors.New("failed to add observabilityaddon into scheme")
		}
		kubeClient = fake.NewClientBuilder().
			WithScheme(s).
			WithStatusSubresource(&oav1beta1.ObservabilityAddon{}).
			Build()
	} else {
		config, err := clientcmd.BuildConfigFromFlags("", "")
		if err != nil {
			return nil, errors.New("failed to create the kube config")
		}
		s := scheme.Scheme
		if err := oav1beta1.AddToScheme(s); err != nil {
			return nil, errors.New("failed to add observabilityaddon into scheme")
		}
		kubeClient, err = client.New(config, client.Options{Scheme: s})
		if err != nil {
			return nil, errors.New("failed to create the kube client")
		}
	}

	logger.Log("msg", "Creating status client", "standalone", standalone, "isUwl", isUwl)

	statusLogger := logr.FromSlogHandler(slog.New(slog.NewTextHandler(os.Stdout, nil)).With("component", "statusclient").Handler())
	return &StatusReport{
		statusClient:   kubeClient,
		standalone:     standalone,
		isUwl:          isUwl,
		statusReporter: status.NewStatus(kubeClient, addonName, addonNamespace, statusLogger),
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

	s.logger.Log("msg", "Updating status", "component", component, "reason", reason, "message", message)

	if err := s.statusReporter.UpdateComponentCondition(ctx, component, reason, message); err != nil {
		return err
	}

	return nil
}
