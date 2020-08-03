// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

const (
	defaultHostport int32 = 3001
	defaultReplicas int32 = 1
)

type GrafanaDatasources struct {
	APIVersion  int                  `json:"apiVersion"`
	Datasources []*GrafanaDatasource `json:"datasources"`
}

type GrafanaDatasource struct {
	Access            string `json:"access"`
	BasicAuth         bool   `json:"basicAuth"`
	BasicAuthPassword string `json:"basicAuthPassword"`
	BasicAuthUser     string `json:"basicAuthUser"`
	Editable          bool   `json:"editable"`
	Name              string `json:"name"`
	OrgID             int    `json:"orgId"`
	Type              string `json:"type"`
	URL               string `json:"url"`
	Version           int    `json:"version"`
}

func newGrafanaConfigSpec() *monitoringv1alpha1.GrafanaSpec {
	spec := &monitoringv1alpha1.GrafanaSpec{}
	spec.Hostport = defaultHostport
	spec.Replicas = defaultReplicas
	return spec
}

func updateGrafanaConfig(mco *monitoringv1alpha1.MultiClusterObservability) {
	if mco.Spec.Grafana.Hostport == 0 {
		mco.Spec.Grafana.Hostport = defaultHostport
	}
	if mco.Spec.Grafana.Replicas == 0 {
		mco.Spec.Grafana.Replicas = defaultReplicas
	}
}

// GenerateGrafanaDataSource is used to generate the GrafanaDatasource as a secret.
// the GrafanaDatasource points to observatorium api gateway service
func GenerateGrafanaDataSource(
	client client.Client,
	scheme *runtime.Scheme,
	mco *monitoringv1alpha1.MultiClusterObservability) (*reconcile.Result, error) {

	grafanaDatasources, err := json.MarshalIndent(GrafanaDatasources{
		APIVersion: 1,
		Datasources: []*GrafanaDatasource{
			{
				Name:   "Observatorium",
				Type:   "prometheus",
				Access: "proxy",
				URL:    "http://" + mco.Name + obsPartoOfName + "-observatorium-api:8080/api/metrics/v1",
			},
		},
	}, "", "    ")
	if err != nil {
		return &reconcile.Result{}, err
	}

	dsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grafana-datasources",
			Namespace: mco.Namespace,
		},
		Type: "Opaque",
		StringData: map[string]string{
			"datasources.yaml": string(grafanaDatasources),
		},
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err = controllerutil.SetControllerReference(mco, dsSecret, scheme); err != nil {
		return &reconcile.Result{}, err
	}

	// Check if this already exists
	grafanaDSFound := &corev1.Secret{}
	err = client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      dsSecret.Name,
			Namespace: dsSecret.Namespace,
		},
		grafanaDSFound,
	)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new grafana datasource secret",
			"dsSecret.Namespace", dsSecret.Namespace,
			"dsSecret.Name", dsSecret.Name,
		)

		err = client.Create(context.TODO(), dsSecret)
		if err != nil {
			return &reconcile.Result{}, err
		}

		// Pod created successfully - don't requeue
		return nil, nil
	} else if err != nil {
		return &reconcile.Result{}, err
	}

	return nil, nil
}
