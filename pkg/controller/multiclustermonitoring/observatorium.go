// Copyright (c) 2020 Red Hat, Inc.

package multiclustermonitoring

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/imdario/mergo"
	observatoriumv1alpha1 "github.com/observatorium/configuration/api/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

const (
	obsPartoOfName = "-observatorium"
	obsAPIGateway  = "observatorium-api"
)

const (
	defaultStorageSize = "1Gi"

	retentionResolution1h  = "1s"
	retentionResolution5m  = "1s"
	retentionResolutionRaw = "14d"

	defaultThanosImage   = "quay.io/thanos/thanos:v0.12.0"
	defaultThanosVersion = "v0.12.0"
)

// GenerateObservatoriumCR returns Observatorium cr defined in MultiClusterMonitoring
func GenerateObservatoriumCR(
	client client.Client,
	scheme *runtime.Scheme,
	monitoring *monitoringv1alpha1.MultiClusterMonitoring) (*reconcile.Result, error) {

	labels := map[string]string{
		"app": monitoring.Name,
	}

	observatoriumCR := &observatoriumv1alpha1.Observatorium{
		ObjectMeta: metav1.ObjectMeta{
			Name:      monitoring.Name + obsPartoOfName,
			Namespace: monitoring.Namespace,
			Labels:    labels,
		},
		Spec: *monitoring.Spec.Observatorium,
	}

	// Set MultiClusterMonitoring instance as the owner and controller
	if err := controllerutil.SetControllerReference(monitoring, observatoriumCR, scheme); err != nil {
		return &reconcile.Result{}, err
	}

	// Check if this Observatorium CR already exists
	observatoriumCRFound := &observatoriumv1alpha1.Observatorium{}
	err := client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      observatoriumCR.Name,
			Namespace: observatoriumCR.Namespace,
		},
		observatoriumCRFound,
	)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new observatorium CR",
			"observatorium.Namespace", observatoriumCR.Namespace,
			"observatorium.Name", observatoriumCR.Name,
		)
		err = client.Create(context.TODO(), observatoriumCR)
		if err != nil {
			return &reconcile.Result{}, err
		}

		// Pod created successfully - don't requeue
		return nil, nil
	} else if err != nil {
		return &reconcile.Result{}, err
	}

	oldSpec := observatoriumCRFound.Spec
	newSpec := observatoriumCR.Spec
	if !reflect.DeepEqual(oldSpec, newSpec) {
		if err := mergo.Merge(&oldSpec, newSpec, mergo.WithOverride); err != nil {
			return &reconcile.Result{}, err
		}
		newObj := observatoriumCRFound.DeepCopy()
		newObj.Spec = oldSpec
		err = client.Update(context.TODO(), newObj)
		if err != nil {
			return &reconcile.Result{}, err
		}
	}

	return nil, nil
}

func createKubeClient() (kubernetes.Interface, error) {
	config, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return kubeClient, err
}

func GenerateAPIGatewayRoute(
	client client.Client,
	scheme *runtime.Scheme,
	monitoring *monitoringv1alpha1.MultiClusterMonitoring) (*reconcile.Result, error) {

	labelSelector := fmt.Sprintf(
		"app.kubernetes.io/component=%s, app.kubernetes.io/instance=%s",
		"api",
		monitoring.Name+obsPartoOfName,
	)

	listOptions := metav1.ListOptions{
		LabelSelector: labelSelector,
	}
	kubeClient, err := createKubeClient()
	if err != nil {
		log.Error(err, "Failed to create kube client")
		return &reconcile.Result{}, err
	}

	apiGatewayServices, err := kubeClient.CoreV1().Services(monitoring.Namespace).List(listOptions)
	if err == nil && len(apiGatewayServices.Items) > 0 {
		apiGateway := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      obsAPIGateway,
				Namespace: monitoring.Namespace,
			},
			Spec: routev1.RouteSpec{
				Port: &routev1.RoutePort{
					TargetPort: intstr.FromString("http"),
				},
				To: routev1.RouteTargetReference{
					Kind: "Service",
					Name: apiGatewayServices.Items[0].GetName(),
				},
			},
		}
		err = client.Get(
			context.TODO(),
			types.NamespacedName{Name: apiGateway.Name, Namespace: apiGateway.Namespace},
			&routev1.Route{})
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating a new route to expose observatorium api",
				"apiGateway.Namespace", apiGateway.Namespace,
				"apiGateway.Name", apiGateway.Name,
			)
			err = client.Create(context.TODO(), apiGateway)
			if err != nil {
				return &reconcile.Result{}, err
			}
		}

	} else if err == nil && len(apiGatewayServices.Items) == 0 {
		log.Info("Cannot find the service ",
			"serviceName",
			monitoring.Name+obsPartoOfName+"-"+obsAPIGateway,
		)
		return &reconcile.Result{RequeueAfter: time.Second * 10}, nil
	} else {
		return &reconcile.Result{}, err
	}
	return nil, nil
}

func newDefaultObservatoriumSpec() *observatoriumv1alpha1.ObservatoriumSpec {
	obs := &observatoriumv1alpha1.ObservatoriumSpec{}

	obs.API.Image = "quay.io/observatorium/observatorium:master-2020-04-29-v0.1.1-14-gceac185"
	obs.API.Version = "master-2020-04-29-v0.1.1-14-gceac185"

	obs.APIQuery.Image = defaultThanosImage
	obs.APIQuery.Version = defaultThanosVersion

	obs.Compact = newCompactSpec()

	obs.Hashrings = []*observatoriumv1alpha1.Hashring{
		{Hashring: "default", Tenants: []string{}},
	}

	obs.ObjectStorageConfig.Name = "thanos-objectstorage"
	obs.ObjectStorageConfig.Key = "thanos.yaml"

	obs.Query.Image = defaultThanosImage
	obs.Query.Version = defaultThanosVersion

	replicas := int32(1)
	obs.QueryCache.Image = "quay.io/cortexproject/cortex:master-fdcd992f"
	obs.QueryCache.Version = "master-fdcd992f"
	obs.QueryCache.Replicas = &replicas

	obs.Receivers = newReceiversSpec()
	obs.Rule = newRuleSpec()
	obs.Store = newStoreSpec()

	obs.ThanosReceiveController.Image = "quay.io/observatorium/thanos-receive-controller:latest"
	obs.ThanosReceiveController.Version = "latest"
	return obs
}

func newReceiversSpec() observatoriumv1alpha1.ReceiversSpec {
	receSpec := observatoriumv1alpha1.ReceiversSpec{}
	receSpec.Image = defaultThanosImage
	receSpec.Version = defaultThanosVersion
	receSpec.VolumeClaimTemplate = newVolumeClaimTemplate(defaultStorageSize)

	return receSpec
}

func newRuleSpec() observatoriumv1alpha1.RuleSpec {
	ruleSpec := observatoriumv1alpha1.RuleSpec{}
	ruleSpec.Image = defaultThanosImage
	ruleSpec.Version = defaultThanosVersion
	ruleSpec.VolumeClaimTemplate = newVolumeClaimTemplate(defaultStorageSize)

	return ruleSpec
}

func newStoreSpec() observatoriumv1alpha1.StoreSpec {
	storeSpec := observatoriumv1alpha1.StoreSpec{}

	storeSpec.Image = defaultThanosImage
	storeSpec.Version = defaultThanosVersion
	storeSpec.VolumeClaimTemplate = newVolumeClaimTemplate(defaultStorageSize)
	shards := int32(1)
	storeSpec.Shards = &shards
	storeSpec.Cache = newStoreCacheSpec()

	return storeSpec
}

func newStoreCacheSpec() observatoriumv1alpha1.StoreCacheSpec {
	storeCacheSpec := observatoriumv1alpha1.StoreCacheSpec{}
	storeCacheSpec.Image = "docker.io/memcached:1.6.3-alpine"
	storeCacheSpec.Version = "1.6.3-alpine"
	storeCacheSpec.ExporterImage = "prom/memcached-exporter:v0.6.0"
	storeCacheSpec.ExporterVersion = "v0.6.0"
	replicas := int32(1)
	storeCacheSpec.Replicas = &replicas
	limit := int32(1024)
	storeCacheSpec.MemoryLimitMB = &limit

	return storeCacheSpec
}

func newCompactSpec() observatoriumv1alpha1.CompactSpec {
	compactSpec := observatoriumv1alpha1.CompactSpec{}
	compactSpec.Image = defaultThanosImage
	compactSpec.Version = defaultThanosVersion
	compactSpec.RetentionResolutionRaw = retentionResolutionRaw
	compactSpec.RetentionResolution5m = retentionResolution5m
	compactSpec.RetentionResolution1h = retentionResolution1h
	compactSpec.VolumeClaimTemplate = newVolumeClaimTemplate(defaultStorageSize)

	return compactSpec
}

func newVolumeClaimTemplate(size string) observatoriumv1alpha1.VolumeClaimTemplate {
	vct := observatoriumv1alpha1.VolumeClaimTemplate{}
	vct.Spec = v1.PersistentVolumeClaimSpec{
		AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse(size),
			},
		},
	}
	return vct
}
