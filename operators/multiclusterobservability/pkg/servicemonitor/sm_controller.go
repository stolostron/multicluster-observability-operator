// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package servicemonitor

import (
	"context"
	"os"
	"reflect"
	"time"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	promclientset "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

const (
	ocpMonitoringNamespace = "openshift-monitoring"
	metricsNamePrefix      = "acm_"
)

var (
	log                    = logf.Log.WithName("sm_controller")
	isSmControllerRunnning = false
)

func Start() {

	if isSmControllerRunnning {
		return
	}
	isSmControllerRunnning = true

	promClient, err := promclientset.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		log.Error(err, "Failed to create prom client")
		os.Exit(1)
	}
	watchlist := cache.NewListWatchFromClient(promClient.MonitoringV1().RESTClient(), "servicemonitors", config.GetDefaultNamespace(),
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&promv1.ServiceMonitor{},
		time.Minute*60,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    onAdd(promClient),
			DeleteFunc: onDelete(promClient),
			UpdateFunc: onUpdate(promClient),
		},
	)

	stop := make(chan struct{})
	go controller.Run(stop)
}

func onAdd(promClient promclientset.Interface) func(obj interface{}) {
	return func(obj interface{}) {
		sm := obj.(*promv1.ServiceMonitor)
		if sm.ObjectMeta.OwnerReferences != nil && sm.ObjectMeta.OwnerReferences[0].Kind == "Observatorium" {
			updateServiceMonitor(promClient, sm)
		}
	}
}

func onDelete(promClient promclientset.Interface) func(obj interface{}) {
	return func(obj interface{}) {
		sm := obj.(*promv1.ServiceMonitor)
		if sm.ObjectMeta.OwnerReferences != nil && sm.ObjectMeta.OwnerReferences[0].Kind == "Observatorium" {
			err := promClient.MonitoringV1().ServiceMonitors(ocpMonitoringNamespace).Delete(context.TODO(), sm.Name, metav1.DeleteOptions{})
			if err != nil {
				log.Error(err, "Failed to delete ServiceMonitor", "namespace", ocpMonitoringNamespace, "name", sm.Name)
			} else {
				log.Info("ServiceMonitor Deleted", "namespace", ocpMonitoringNamespace, "name", sm.Name)
			}
		}
	}
}

func onUpdate(promClient promclientset.Interface) func(newObj interface{}, oldObj interface{}) {
	return func(newObj interface{}, oldObj interface{}) {
		newSm := newObj.(*promv1.ServiceMonitor)
		oldSm := oldObj.(*promv1.ServiceMonitor)
		if newSm.ObjectMeta.OwnerReferences != nil && newSm.ObjectMeta.OwnerReferences[0].Kind == "Observatorium" &&
			!reflect.DeepEqual(newSm.Spec, oldSm.Spec) {
			updateServiceMonitor(promClient, newSm)
		}
	}
}

func updateServiceMonitor(promClient promclientset.Interface, sm *promv1.ServiceMonitor) {
	found, err := promClient.MonitoringV1().ServiceMonitors(ocpMonitoringNamespace).Get(context.TODO(), sm.Name, metav1.GetOptions{})
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
	endpoints := []promv1.Endpoint{}
	for _, endpoint := range sm.Spec.Endpoints {
		metricsRelabels := endpoint.MetricRelabelConfigs
		if metricsRelabels == nil {
			metricsRelabels = []*promv1.RelabelConfig{}
		}
		metricsRelabels = append(metricsRelabels, &promv1.RelabelConfig{
			SourceLabels: []string{"__name__"},
			Regex:        "(.+)",
			TargetLabel:  "__name__",
			Replacement:  metricsNamePrefix + "${1}",
		})
		endpoint.MetricRelabelConfigs = metricsRelabels
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
