// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"bytes"
	"context"
	"fmt"
	"reflect"

	oauthv1 "github.com/openshift/api/oauth/v1"
	routev1 "github.com/openshift/api/route/v1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

const (
	defaultReplicas int32 = 1
	restartLabel          = "datasource/time-restarted"
	datasourceKey         = "datasources.yaml"

	haProxyRouterTimeoutKey     = "haproxy.router.openshift.io/timeout"
	defaultHaProxyRouterTimeout = "300s"
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
	TLSAuth   bool `yaml:"tlsAuth,omitempty"`
	TLSAuthCA bool `yaml:"tlsAuthWithCACert,omitempty"`
	// Timeout is the request timeout in seconds for an HTTP datasource.
	Timeout               string `yaml:"timeout,omitempty"`
	HttpMethod            string `yaml:"httpMethod,omitempty"`
	TimeInterval          string `yaml:"timeInterval,omitempty"`
	CustomQueryParameters string `yaml:"customQueryParameters,omitempty"`
}

type SecureJsonData struct {
	TLSCACert     string `yaml:"tlsCACert,omitempty"`
	TLSClientCert string `yaml:"tlsClientCert,omitempty"`
	TLSClientKey  string `yaml:"tlsClientKey,omitempty"`
}

// GenerateGrafanaDataSource is used to generate the GrafanaDatasource as a secret.
// the GrafanaDatasource points to observatorium api gateway service.
func GenerateGrafanaDataSource(
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {

	DynamicTimeInterval := mco.Spec.ObservabilityAddonSpec.Interval

	if DynamicTimeInterval > 30 {
		DynamicTimeInterval = 30
	}

	grafanaDatasources, err := yaml.Marshal(GrafanaDatasources{
		APIVersion: 1,
		Datasources: []*GrafanaDatasource{
			{
				Name:      "Observatorium",
				Type:      "prometheus",
				Access:    "proxy",
				IsDefault: true,
				URL: fmt.Sprintf(
					"http://%s.%s.svc.cluster.local:8080",
					config.ProxyServiceName,
					config.GetDefaultNamespace(),
				),
				JSONData: &JsonData{
					Timeout:               "300",
					CustomQueryParameters: "max_source_resolution=auto",
					TimeInterval:          fmt.Sprintf("%ds", mco.Spec.ObservabilityAddonSpec.Interval),
				},
			},
			{
				Name:      "Observatorium-Dynamic",
				Type:      "prometheus",
				Access:    "proxy",
				IsDefault: false,
				URL: fmt.Sprintf(
					"http://%s.%s.svc.cluster.local:8080",
					config.ProxyServiceName,
					config.GetDefaultNamespace(),
				),
				JSONData: &JsonData{
					Timeout:               "300",
					CustomQueryParameters: "max_source_resolution=auto",
					TimeInterval:          fmt.Sprintf("%ds", DynamicTimeInterval),
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
		Data: map[string][]byte{
			datasourceKey: grafanaDatasources,
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
	if (grafanaDSFound.Data[datasourceKey] != nil &&
		!bytes.Equal(grafanaDSFound.Data[datasourceKey], dsSecret.Data[datasourceKey])) ||
		grafanaDSFound.Data[datasourceKey] == nil {
		log.Info("Updating grafana datasource secret")
		err = c.Update(context.TODO(), dsSecret)
		if err != nil {
			log.Error(err, "Failed to update grafana datasource secret")
			return &ctrl.Result{}, err
		}
		err = util.UpdateDeployLabel(c, config.GetOperandName(config.Grafana),
			config.GetDefaultNamespace(), restartLabel)
		if err != nil {
			return &ctrl.Result{}, err
		}
	}

	return nil, nil
}

func GenerateGrafanaRoute(
	c client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {
	grafanaRoute := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.GrafanaRouteName,
			Namespace: config.GetDefaultNamespace(),
			Annotations: map[string]string{
				haProxyRouterTimeoutKey: defaultHaProxyRouterTimeout,
			},
		},
		Spec: routev1.RouteSpec{
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString("oauth-proxy"),
			},
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: config.GrafanaServiceName,
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationReencrypt,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
			},
		},
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, grafanaRoute, scheme); err != nil {
		return &ctrl.Result{}, err
	}

	found := &routev1.Route{}
	err := c.Get(
		context.TODO(),
		types.NamespacedName{Name: grafanaRoute.Name, Namespace: grafanaRoute.Namespace},
		found,
	)
	if err != nil && errors.IsNotFound(err) {
		log.Info(
			"Creating a new route to expose grafana",
			"grafanaRoute.Namespace",
			grafanaRoute.Namespace,
			"grafanaRoute.Name",
			grafanaRoute.Name,
		)
		err = c.Create(context.TODO(), grafanaRoute)
		if err != nil {
			return &ctrl.Result{}, err
		}
		return nil, nil
	}

	// if no annotations are set, set the default timeout
	if found.Annotations == nil {
		found.Annotations = map[string]string{}
		found.Annotations[haProxyRouterTimeoutKey] = defaultHaProxyRouterTimeout
	}

	// if some annotations are set, but the timeout is not set, set the default timeout
	// otherwise, use the existing timeout which allows for custom timeouts.
	// we do not want to overwrite other labels that may be set.
	if _, ok := found.Annotations[haProxyRouterTimeoutKey]; !ok {
		found.Annotations[haProxyRouterTimeoutKey] = defaultHaProxyRouterTimeout
	}

	if !reflect.DeepEqual(found.Spec, grafanaRoute.Spec) {
		found.Spec = grafanaRoute.Spec
	}

	err = c.Update(context.TODO(), found)
	if err != nil {
		log.Error(
			err,
			"failed update for Grafana Route",
			"grafanaRoute.Name",
			grafanaRoute.Name,
		)
		return &ctrl.Result{}, err
	}
	return nil, nil
}

func GenerateGrafanaOauthClient(
	c client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {
	host, err := config.GetRouteHost(c, config.GrafanaRouteName, config.GetDefaultNamespace())
	if err != nil {
		return nil, err
	}
	oauthClient := &oauthv1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name: config.GrafanaOauthClientName,
		},
		Secret:       config.GrafanaOauthClientSecret,
		RedirectURIs: []string{"https://" + host},
		GrantMethod:  oauthv1.GrantHandlerAuto,
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, oauthClient, scheme); err != nil {
		return &ctrl.Result{}, err
	}

	found := &oauthv1.OAuthClient{}
	err = c.Get(
		context.TODO(),
		types.NamespacedName{Name: config.GrafanaOauthClientName},
		found,
	)
	if err != nil && errors.IsNotFound(err) {
		log.Info(
			"Creating a new oauthclient for grafana",
			"GrafanaOauthClientName",
			config.GrafanaOauthClientName,
		)
		err = c.Create(context.TODO(), oauthClient)
		if err != nil {
			return &ctrl.Result{}, err
		}
		return nil, nil
	}
	return nil, nil
}

func DeleteGrafanaOauthClient(c client.Client) error {
	found := &oauthv1.OAuthClient{}
	err := c.Get(
		context.TODO(),
		types.NamespacedName{Name: config.GrafanaOauthClientName},
		found,
	)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		} else {
			return nil
		}
	}
	err = c.Delete(context.TODO(), found, &client.DeleteOptions{})
	return err
}
