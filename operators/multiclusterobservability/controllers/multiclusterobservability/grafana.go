// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"context"
	"fmt"
	"reflect"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

const (
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
	IsDefault         bool            `yaml:"isDefault"`
	Name              string          `yaml:"name"`
	OrgID             int             `yaml:"orgId"`
	Type              string          `yaml:"type"`
	URL               string          `yaml:"url"`
	Version           int             `yaml:"version"`
	JSONData          *JsonData       `yaml:"jsonData"`
	SecureJSONData    *SecureJsonData `yaml:"secureJsonData"`
}

type JsonData struct {
	TLSAuth      bool   `yaml:"tlsAuth"`
	TLSAuthCA    bool   `yaml:"tlsAuthWithCACert"`
	QueryTimeout string `yaml:"queryTimeout"`
	HttpMethod   string `yaml:"httpMethod"`
}

type SecureJsonData struct {
	TLSCACert     string `yaml:"tlsCACert"`
	TLSClientCert string `yaml:"tlsClientCert"`
	TLSClientKey  string `yaml:"tlsClientKey"`
}

// GenerateGrafanaDataSource is used to generate the GrafanaDatasource as a secret.
// the GrafanaDatasource points to observatorium api gateway service
func GenerateGrafanaDataSource(
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {

	grafanaDatasources, err := yaml.Marshal(GrafanaDatasources{
		APIVersion: 1,
		Datasources: []*GrafanaDatasource{
			{
				Name:      "Observatorium",
				Type:      "prometheus",
				Access:    "proxy",
				IsDefault: true,
				URL:       fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", config.ProxyServiceName, config.GetDefaultNamespace()),
				JSONData: &JsonData{
					QueryTimeout: "300s",
				},
			},
		},
	})
	if err != nil {
		return &ctrl.Result{}, err
	}

	dsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grafana-datasources",
			Namespace: config.GetDefaultNamespace(),
		},
		Type: "Opaque",
		StringData: map[string]string{
			"datasources.yaml": string(grafanaDatasources),
		},
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err = controllerutil.SetControllerReference(mco, dsSecret, scheme); err != nil {
		return &ctrl.Result{}, err
	}

	// Check if this already exists
	grafanaDSFound := &corev1.Secret{}
	err = c.Get(
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

		err = c.Create(context.TODO(), dsSecret)
		if err != nil {
			return &ctrl.Result{}, err
		}

		// Pod created successfully - don't requeue
		return nil, nil
	} else if err != nil {
		return &ctrl.Result{}, err
	}

	if !reflect.DeepEqual(grafanaDSFound.Data, dsSecret.Data) {
		log.Info("Updating grafana datasource secret")
		dsSecret.ObjectMeta.ResourceVersion = grafanaDSFound.ObjectMeta.ResourceVersion
		err = c.Update(context.TODO(), dsSecret)
		if err != nil {
			log.Error(err, "Failed to update grafana datasource secret")
			return &ctrl.Result{}, err
		}
	}

	return nil, nil
}
