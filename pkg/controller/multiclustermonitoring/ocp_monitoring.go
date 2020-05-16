// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	routev1ClientSet "github.com/openshift/client-go/route/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

func createRoutev1Client() (routev1ClientSet.Interface, error) {
	config, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	routev1Client, err := routev1ClientSet.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return routev1Client, err
}

func UpdateOCPMonitoringCM(monitoring *monitoringv1alpha1.MultiClusterMonitoring) (*reconcile.Result, error) {

	routev1Client, err := createRoutev1Client()
	if err != nil {
		log.Error(err, "Failed to create routev1 client")
		return &reconcile.Result{}, nil
	}

	// Try to get route instance
	obsRoute, err := routev1Client.RouteV1().Routes(monitoring.Namespace).Get(obsAPIGateway, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to get route", obsAPIGateway)
		return &reconcile.Result{}, err
	}

	remoteRriteURL := obsRoute.Spec.Host

	if remoteRriteURL == "" {
		remoteRriteURL = monitoring.Name + obsPartoOfName + "-" + obsAPIGateway + "." + monitoring.Namespace + ".svc"
	}

	err = util.UpdateHubClusterMonitoringConfig(remoteRriteURL)
	if err != nil {
		return &reconcile.Result{}, err
	}

	return nil, nil
}
