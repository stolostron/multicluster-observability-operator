// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"context"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
)

const (
	defaultHostport int32 = 3001
	defaultReplicas int32 = 1
)

type GrafanaDatasources struct {
	APIVersion  int                  `yaml:"apiVersion"`
	Datasources []*GrafanaDatasource `yaml:"datasources"`
}

type GrafanaDatasource struct {
	Access            string          `yaml:"access"`
	BasicAuth         bool            `yaml:"basicAuth"`
	BasicAuthPassword string          `yaml:"basicAuthPassword"`
	BasicAuthUser     string          `yaml:"basicAuthUser"`
	Editable          bool            `yaml:"editable"`
	Name              string          `yaml:"name"`
	OrgID             int             `yaml:"orgId"`
	Type              string          `yaml:"type"`
	URL               string          `yaml:"url"`
	Version           int             `yaml:"version"`
	JSONData          *JsonData       `yaml:"jsonData"`
	SecureJSONData    *SecureJsonData `yaml:"secureJsonData"`
}

type JsonData struct {
	TLSAuth   bool `yaml:"tlsAuth"`
	TLSAuthCA bool `yaml:"tlsAuthWithCACert"`
}

type SecureJsonData struct {
	TLSCACert     string `yaml:"tlsCACert"`
	TLSClientCert string `yaml:"tlsClientCert"`
	TLSClientKey  string `yaml:"tlsClientKey"`
}

// GenerateGrafanaDataSource is used to generate the GrafanaDatasource as a secret.
// the GrafanaDatasource points to observatorium api gateway service
func GenerateGrafanaDataSource(
	client client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {

	grafanaDatasources, err := yaml.Marshal(GrafanaDatasources{
		APIVersion: 1,
		Datasources: []*GrafanaDatasource{
			{
				Name:   "Observatorium",
				Type:   "prometheus",
				Access: "proxy",
				URL:    "http://rbac-query-proxy." + mco.Namespace + ".svc.cluster.local:8080",
			},
		},
	})
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
