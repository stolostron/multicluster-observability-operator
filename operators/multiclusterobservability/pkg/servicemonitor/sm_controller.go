// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package servicemonitor

import (
	"context"
	"os"
	"time"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	promclientset "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	metricsNamePrefix = "acm_"
)

var (
	ocpMonitoringNamespace = config.GetDefaultNamespace()
	log                    = logf.Log.WithName("sm_controller")
	isSmControllerRunning  = false
)

func Start() {
	if isSmControllerRunning {
		return
	}
	isSmControllerRunning = true

	promClient, err := promclientset.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		log.Error(err, "Failed to create prom client")
		os.Exit(1)
	}
	watchlist := cache.NewListWatchFromClient(
		promClient.MonitoringV1().RESTClient(),
		"servicemonitors",
		config.GetDefaultNamespace(),
		fields.Everything(),
	)
	options := cache.InformerOptions{
		ListerWatcher: watchlist,
		ObjectType:    &promv1.ServiceMonitor{},
		ResyncPeriod:  time.Minute * 60,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    onAdd(promClient),
			UpdateFunc: onUpdate(promClient),
		},
	}
	_, controller := cache.NewInformerWithOptions(options)

	stop := make(chan struct{})
	go controller.Run(stop)
}

func onAdd(promClient promclientset.Interface) func(obj any) {
	return func(obj any) {
		sm := obj.(*promv1.ServiceMonitor)
		if sm.OwnerReferences != nil && sm.ObjectMeta.OwnerReferences[0].Kind == "Observatorium" {
			updateServiceMonitor(promClient, sm)
		}
	}
}

func onUpdate(promClient promclientset.Interface) func(oldObj any, newObj any) {
	return func(oldObj any, newObj any) {
		newSm := newObj.(*promv1.ServiceMonitor)
		oldSm := oldObj.(*promv1.ServiceMonitor)
		if newSm.OwnerReferences != nil && newSm.ObjectMeta.OwnerReferences[0].Kind == "Observatorium" &&
			!equality.Semantic.DeepEqual(newSm.Spec, oldSm.Spec) {
			updateServiceMonitor(promClient, newSm)
		}
	}
}

func updateServiceMonitor(promClient promclientset.Interface, sm *promv1.ServiceMonitor) {
	found, err := promClient.MonitoringV1().
		ServiceMonitors(ocpMonitoringNamespace).
		Get(context.TODO(), sm.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err := promClient.MonitoringV1().ServiceMonitors(ocpMonitoringNamespace).Create(context.TODO(),
				rewriteLabels(sm, ""), metav1.CreateOptions{})
			if err != nil {
				log.Error(err, "Failed to create ServiceMonitor", "namespace", ocpMonitoringNamespace, "name", sm.Name)
			} else {
				log.Info("ServiceMonitor Created", "namespace", ocpMonitoringNamespace, "name", sm.Name)
			}
		} else {
			log.Error(err, "Failed to check ServiceMonitor", "namespace", ocpMonitoringNamespace, "name", sm.Name)
		}
		return
	}
	_, err = promClient.MonitoringV1().ServiceMonitors(ocpMonitoringNamespace).Update(context.TODO(),
		rewriteLabels(sm, found.ResourceVersion), metav1.UpdateOptions{})
	if err != nil {
		log.Error(err, "Failed to update ServiceMonitor", "namespace", ocpMonitoringNamespace, "name", sm.Name)
	} else {
		log.Info("ServiceMonitor Updated", "namespace", ocpMonitoringNamespace, "name", sm.Name)
	}
}

func rewriteLabels(sm *promv1.ServiceMonitor, resourceVersion string) *promv1.ServiceMonitor {
	update := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sm.Name,
			Namespace: ocpMonitoringNamespace,
		},
	}
	replacement := metricsNamePrefix + "${1}"
	endpoints := make([]promv1.Endpoint, 0, len(sm.Spec.Endpoints))
	for _, endpoint := range sm.Spec.Endpoints {
		if endpoint.MetricRelabelConfigs == nil {
			metricsRelabels := make([]promv1.RelabelConfig, 0, 1)
			metricsRelabels = append(metricsRelabels, promv1.RelabelConfig{
				SourceLabels: []promv1.LabelName{"__name__"},
				Regex:        "(.+)",
				TargetLabel:  "__name__",
				Replacement:  &replacement,
			})
			endpoint.MetricRelabelConfigs = metricsRelabels
		}
		endpoints = append(endpoints, endpoint)
	}
	sm.Spec.Endpoints = endpoints
	sm.Spec.NamespaceSelector = promv1.NamespaceSelector{
		MatchNames: []string{config.GetDefaultNamespace()},
	}
	update.Spec = sm.Spec
	update.ResourceVersion = resourceVersion
	return update
}
