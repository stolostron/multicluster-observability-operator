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
	APIVersion  int                  `yaml:"apiVersion,omitempty"`
	Datasources []*GrafanaDatasource `yaml:"datasources,omitempty"`
}

type GrafanaDatasource struct {
	Access            string          `yaml:"access,omitempty"`
	BasicAuth         bool            `yaml:"basicAuth,omitempty"`
	BasicAuthPassword string          `yaml:"basicAuthPassword,omitempty"`
	BasicAuthUser     string          `yaml:"basicAuthUser,omitempty"`
	Editable          bool            `yaml:"editable,omitempty"`
	IsDefault         bool            `yaml:"isDefault,omitempty"`
	Name              string          `yaml:"name,omitempty"`
	OrgID             int             `yaml:"orgId,omitempty"`
	Type              string          `yaml:"type,omitempty"`
	URL               string          `yaml:"url,omitempty"`
	Version           int             `yaml:"version,omitempty"`
	JSONData          *JsonData       `yaml:"jsonData,omitempty"`
	SecureJSONData    *SecureJsonData `yaml:"secureJsonData,omitempty"`
}

type JsonData struct {
	TLSAuth      bool   `yaml:"tlsAuth,omitempty"`
	TLSAuthCA    bool   `yaml:"tlsAuthWithCACert,omitempty"`
	QueryTimeout string `yaml:"queryTimeout,omitempty"`
	HttpMethod   string `yaml:"httpMethod,omitempty"`
}

type SecureJsonData struct {
	TLSCACert     string `yaml:"tlsCACert,omitempty"`
	TLSClientCert string `yaml:"tlsClientCert,omitempty"`
	TLSClientKey  string `yaml:"tlsClientKey,omitempty"`
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
