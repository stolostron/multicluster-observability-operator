// Copyright (c) 2020 Red Hat, Inc.

package config

import (
	"context"

	ocinfrav1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	clusterNameLabelKey      = "cluster"
	obsAPIGateway            = "observatorium-api"
	infrastructureConfigName = "cluster"
	defaultNamespace         = "open-cluster-management-observability"
	defaultTenantName        = "prod"

	AnnotationKeyImageRepository = "mco-imageRepository"
	AnnotationKeyImageTagSuffix  = "mco-imageTagSuffix"

	DefaultImgPullPolicy = corev1.PullAlways
	DefaultImgPullSecret = "multiclusterhub-operator-pull-secret"
	DefaultImgRepository = "quay.io/open-cluster-management"
	DefaultImgTagSuffix  = "latest"
	DefaultStorageClass  = "gp2"
	DefaultStorageSize   = "10Gi"

	DefaultRetentionResolution1h  = "30d"
	DefaultRetentionResolution5m  = "14d"
	DefaultRetentionResolutionRaw = "5d"

	GrafanaImgRepo      = "grafana"
	GrafanaImgTagSuffix = "7.1.3"

	ObservatoriumImgRepo      = "quay.io/observatorium"
	ObservatoriumImgTagSuffix = "master-2020-08-06-10069f8"

	EndpointControllerImgTagSuffix = "0.1.0-bc96d1251892a6de3a382164f6e7944ce3028d28"

	MetricsCollectorImgTagSuffix = "2.1.0-1aa917b69ceb64c5a77b999ffb69529aa6fb069c"

	DefaultAddonInterval = 60
)

var log = logf.Log.WithName("config")

var monitoringCRName = ""

// GetClusterNameLabelKey returns the key for the injected label
func GetClusterNameLabelKey() string {
	return clusterNameLabelKey
}

func IsNeededReplacement(annotations map[string]string) bool {
	if annotations != nil {
		_, hasRepo := annotations[AnnotationKeyImageRepository]
		_, hasTagSuffix := annotations[AnnotationKeyImageTagSuffix]
		return hasRepo && hasTagSuffix
	}
	return false
}

// GetDefaultTenantName returns the default tenant name
func GetDefaultTenantName() string {
	return defaultTenantName
}

// GetObsAPIUrl is used to get the URL for observartium api gateway
func GetObsAPIUrl(client client.Client, namespace string) (string, error) {
	found := &routev1.Route{}

	err := client.Get(context.TODO(), types.NamespacedName{Name: obsAPIGateway, Namespace: namespace}, found)
	if err != nil {
		return "", err
	}
	return found.Spec.Host, nil
}

func GetDefaultNamespace() string {
	return defaultNamespace
}

// GetMonitoringCRName returns monitoring cr name
func GetMonitoringCRName() string {
	return monitoringCRName
}

// SetMonitoringCRName sets the cr name
func SetMonitoringCRName(crName string) {
	monitoringCRName = crName
}

func infrastructureConfigNameNsN() types.NamespacedName {
	return types.NamespacedName{
		Name: infrastructureConfigName,
	}
}

// GetKubeAPIServerAddress is used to get the api server url
func GetKubeAPIServerAddress(client client.Client) (string, error) {
	infraConfig := &ocinfrav1.Infrastructure{}
	if err := client.Get(context.TODO(), infrastructureConfigNameNsN(), infraConfig); err != nil {
		return "", err
	}

	return infraConfig.Status.APIServerURL, nil
}

// GetClusterID is used to get the cluster uid
func GetClusterID(ocpClient ocpClientSet.Interface) (string, error) {
	clusterVersion, err := ocpClient.ConfigV1().ClusterVersions().Get("version", v1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to get clusterVersion")
		return "", err
	}

	return string(clusterVersion.Spec.ClusterID), nil
}
