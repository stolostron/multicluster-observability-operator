// Copyright Contributors to the Open Cluster Management project

package status

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-observability-operator/collectors/metrics/pkg/logger"
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
		kubeClient = fake.NewFakeClient()
	} else if standaloneMode {
		kubeClient = nil
	} else {
		config, err := clientcmd.BuildConfigFromFlags("", "")
		if err != nil {
			return nil, errors.New("Failed to create the kube config")
		}
		s := scheme.Scheme
		if err := oav1beta1.AddToScheme(s); err != nil {
			return nil, errors.New("Failed to add observabilityaddon into scheme")
		}
		kubeClient, err = client.New(config, client.Options{Scheme: s})
		if err != nil {
			return nil, errors.New("Failed to create the kube client")
		}
	}

	return &StatusReport{
		statusClient: kubeClient,
		logger:       log.With(logger, "component", "statusclient"),
	}, nil
}

func (s *StatusReport) UpdateStatus(t string, r string, m string) error {
	if s.statusClient == nil {
		return nil
	}
	isUwl := false
	if os.Getenv("FROM") == uwlPromURL {
		isUwl = true
	}
	addon := &oav1beta1.ObservabilityAddon{}
	err := s.statusClient.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, addon)
	if err != nil {
		logger.Log(s.logger, logger.Error, "err", err)
		return err
	}
	update := false
	found := false
	conditions := []oav1beta1.StatusCondition{}
	latestC := oav1beta1.StatusCondition{}
	message, conditionType, reason := mergeCondtion(isUwl, t, r, m,
		addon.Status.Conditions[len(addon.Status.Conditions)-1])
	for _, c := range addon.Status.Conditions {
		if c.Status == metav1.ConditionTrue {
			if c.Type != conditionType {
				c.Status = metav1.ConditionFalse
			} else {
				found = true
				if c.Reason != reason || c.Message != message {
					c.Reason = reason
					c.Message = message
					c.LastTransitionTime = metav1.NewTime(time.Now())
					update = true
					latestC = c
					continue
				}
			}
		} else {
			if c.Type == conditionType {
				found = true
				c.Status = metav1.ConditionTrue
				c.Reason = reason
				c.Message = message
				c.LastTransitionTime = metav1.NewTime(time.Now())
				update = true
				latestC = c
				continue
			}
		}
		conditions = append(conditions, c)
	}
	if update {
		conditions = append(conditions, latestC)
	}
	if !found {
		conditions = append(conditions, oav1beta1.StatusCondition{
			Type:               conditionType,
			Status:             metav1.ConditionTrue,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
		update = true
	}
	if update {
		addon.Status.Conditions = conditions
		err = s.statusClient.Status().Update(context.TODO(), addon)
		if err != nil {
			logger.Log(s.logger, logger.Error, "err", err)
		}
		return err
	}
	return nil
}

func mergeCondtion(isUwl bool, t, r, m string, condition oav1beta1.StatusCondition) (string, string, string) {
	messages := strings.Split(condition.Message, " ; ")
	if len(messages) == 1 {
		messages = append(messages, "")
	}
	if isUwl {
		messages[1] = fmt.Sprintf("User Workload: %s", m)
	} else {
		messages[0] = m
	}
	message := strings.Join(messages, " ; ")
	conditionType := "Available"
	reason := "Available"
	if strings.Contains(message, "Failed") {
		conditionType = "Degraded"
		reason = "Degraded"
	}
	return message, conditionType, reason
}
