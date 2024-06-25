// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package status

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-kit/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
)

const (
	name       = "observability-addon"
	namespace  = "open-cluster-management-addon-observability"
	uwlPromURL = "https://prometheus-user-workload.openshift-user-workload-monitoring.svc:9092"
)

type StatusReport struct {
	statusClient client.Client
	logger       log.Logger
}

func New(logger log.Logger) (*StatusReport, error) {
	testMode := os.Getenv("UNIT_TEST") != ""
	standaloneMode := os.Getenv("STANDALONE") == "true"
	var kubeClient client.Client
	if testMode {
		kubeClient = fake.NewClientBuilder().Build()
	} else if standaloneMode {
		kubeClient = nil
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

	return &StatusReport{
		statusClient: kubeClient,
		logger:       log.With(logger, "component", "statusclient"),
	}, nil
}

func (s *StatusReport) UpdateStatus(ctx context.Context, t string, m string) error {
	// statusClient is nil when running on the hub.
	if s.statusClient == nil {
		return nil
	}

	isUwl := false
	if strings.Contains(os.Getenv("FROM"), uwlPromURL) {
		isUwl = true
	}

	retryErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		addon := &oav1beta1.ObservabilityAddon{}
		err := s.statusClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, addon)
		if err != nil {
			return fmt.Errorf("failed to get ObservabilityAddon %s/%s: %w", namespace, name, err)
		}

		// Sort the conditions by rising LastTransitionTime
		sort.Slice(addon.Status.Conditions, func(i, j int) bool {
			return addon.Status.Conditions[i].LastTransitionTime.Before(&addon.Status.Conditions[j].LastTransitionTime)
		})

		currentCondition := addon.Status.Conditions[len(addon.Status.Conditions)-1]
		newCondition := mergeCondtion(isUwl, m, currentCondition)

		// If the current condition is the same, do not update
		if currentCondition.Type == newCondition.Type && currentCondition.Reason == newCondition.Reason && currentCondition.Message == newCondition.Message && currentCondition.Status == newCondition.Status {
			return nil
		}

		s.logger.Log("msg", fmt.Sprintf("Updating status of ObservabilityAddon %s/%s", namespace, name), "type", newCondition.Type, "status", newCondition.Status, "reason", newCondition.Reason)

		// Reset the status of other main conditions
		for i := range addon.Status.Conditions {
			if addon.Status.Conditions[i].Type == "Available" || addon.Status.Conditions[i].Type == "Degraded" || addon.Status.Conditions[i].Type == "Progressing" {
				addon.Status.Conditions[i].Status = metav1.ConditionFalse
			}
		}

		// Set the new condition
		addon.Status.Conditions = mutateOrAppend(addon.Status.Conditions, newCondition)

		if err := s.statusClient.Status().Update(ctx, addon); err != nil {
			return fmt.Errorf("failed to update ObservabilityAddon %s/%s: %w", namespace, name, err)
		}

		return nil
	})
	if retryErr != nil {
		return retryErr
	}
	return nil
}

func mergeCondtion(isUwl bool, m string, condition oav1beta1.StatusCondition) oav1beta1.StatusCondition {
	messages := strings.Split(condition.Message, " ; ")
	if len(messages) == 1 {
		messages = append(messages, "")
	}
	if isUwl {
		messages[1] = fmt.Sprintf("User Workload: %s", m)
	} else {
		messages[0] = m
	}
	message := messages[0]
	if messages[1] != "" {
		message = strings.Join(messages, " ; ")
	}
	conditionType := "Available"
	reason := "Available"
	if strings.Contains(message, "Failed") {
		conditionType = "Degraded"
		reason = "Degraded"
	}
	return oav1beta1.StatusCondition{
		Type:               conditionType,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
	}
}

// mutateOrAppend updates the status conditions with the new condition.
// If the condition already exists, it updates it with the new condition.
// If the condition does not exist, it appends the new condition to the status conditions.
func mutateOrAppend(conditions []oav1beta1.StatusCondition, newCondition oav1beta1.StatusCondition) []oav1beta1.StatusCondition {
	if len(conditions) == 0 {
		return []oav1beta1.StatusCondition{newCondition}
	}

	for i, condition := range conditions {
		if condition.Type == newCondition.Type {
			// Update the existing condition
			conditions[i] = newCondition
			return conditions
		}
	}
	// If the condition type does not exist, append the new condition
	return append(conditions, newCondition)
}
