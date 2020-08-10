// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"context"
	"reflect"
	"time"

	observatoriumv1alpha1 "github.com/observatorium/deployments/operator/api/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
)

const (
	obsPartoOfName = "-observatorium"
	obsAPIGateway  = "observatorium-api"
)

const (
	defaultStorageSize = "1Gi"

	retentionResolution1h  = "30d"
	retentionResolution5m  = "14d"
	retentionResolutionRaw = "5d"

	defaultThanosImage   = "quay.io/thanos/thanos:master-2020-05-24-079ad427"
	defaultThanosVersion = "master-2020-05-24-079ad427"
)

// GenerateObservatoriumCR returns Observatorium cr defined in MultiClusterObservability
func GenerateObservatoriumCR(
	client client.Client, scheme *runtime.Scheme,
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {

	labels := map[string]string{
		"app": mco.Name,
	}

	observatoriumCR := &observatoriumv1alpha1.Observatorium{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mco.Name + obsPartoOfName,
			Namespace: mco.Namespace,
			Labels:    labels,
		},
		Spec: *newDefaultObservatoriumSpec(),
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, observatoriumCR, scheme); err != nil {
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
			"observatorium", observatoriumCR,
		)
		err = client.Create(context.TODO(), observatoriumCR)
		if err != nil {
			return &reconcile.Result{}, err
		}
		return nil, nil
	} else if err != nil {
		return &reconcile.Result{}, err
	}

	oldSpec := observatoriumCRFound.Spec
	newSpec := observatoriumCR.Spec
	if !reflect.DeepEqual(oldSpec, newSpec) {
		newObj := observatoriumCRFound.DeepCopy()
		newObj.Spec = newSpec
		err = client.Update(context.TODO(), newObj)
		if err != nil {
			return &reconcile.Result{}, err
		}
	}

	return nil, nil
}

func GenerateAPIGatewayRoute(
	runclient client.Client, scheme *runtime.Scheme,
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {

	listOptions := []client.ListOption{
		client.MatchingLabels(map[string]string{
			"app.kubernetes.io/component": "api",
			"app.kubernetes.io/instance":  mco.Name + obsPartoOfName,
		}),
	}
	apiGatewayServices := &v1.ServiceList{}
	err := runclient.List(context.TODO(), apiGatewayServices, listOptions...)
	if err == nil && len(apiGatewayServices.Items) > 0 {
		apiGateway := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      obsAPIGateway,
				Namespace: mco.Namespace,
			},
			Spec: routev1.RouteSpec{
				Port: &routev1.RoutePort{
					TargetPort: intstr.FromString("remote-write"),
				},
				To: routev1.RouteTargetReference{
					Kind: "Service",
					//Name: apiGatewayServices.Items[0].GetName(),
					Name: mco.Name + "-observatorium-thanos-receive",
				},
			},
		}
		err = runclient.Get(
			context.TODO(),
			types.NamespacedName{Name: apiGateway.Name, Namespace: apiGateway.Namespace},
			&routev1.Route{})
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating a new route to expose observatorium api",
				"apiGateway.Namespace", apiGateway.Namespace,
				"apiGateway.Name", apiGateway.Name,
			)
			err = runclient.Create(context.TODO(), apiGateway)
			if err != nil {
				return &reconcile.Result{}, err
			}
		}

	} else if err == nil && len(apiGatewayServices.Items) == 0 {
		log.Info("Cannot find the service ",
			"serviceName",
			mco.Name+obsPartoOfName+"-"+obsAPIGateway,
		)
		return &reconcile.Result{RequeueAfter: time.Second * 10}, nil
	} else {
		return &reconcile.Result{}, err
	}
	return nil, nil
}

func newDefaultObservatoriumSpec() *observatoriumv1alpha1.ObservatoriumSpec {
	obs := &observatoriumv1alpha1.ObservatoriumSpec{}

	obs.API.Image = "quay.io/observatorium/observatorium:latest"
	obs.API.Version = "latest"
	obs.API.RBAC = newAPIRBAC()
	obs.API.Tenants = newAPITenants()

	obs.APIQuery.Image = defaultThanosImage
	obs.APIQuery.Version = defaultThanosVersion

	obs.Compact = newCompactSpec()

	obs.Hashrings = []*observatoriumv1alpha1.Hashring{
		{Hashring: "default", Tenants: []string{}},
	}

	obs.ObjectStorageConfig.Thanos = &observatoriumv1alpha1.ObjectStorageConfigSpec{
		Name: "thanos-objectstorage",
		Key:  "thanos.yaml",
	}

	obs.Query.Image = defaultThanosImage
	obs.Query.Version = defaultThanosVersion

	replicas := int32(1)
	obs.QueryCache.Image = "quay.io/cortexproject/cortex:master-fdcd992f"
	obs.QueryCache.Version = "master-fdcd992f"
	obs.QueryCache.Replicas = &replicas

	obs.Receivers = newReceiversSpec()
	obs.Rule = newRuleSpec()
	obs.Store = newStoreSpec()

	obs.ThanosReceiveController.Image = "quay.io/observatorium/thanos-receive-controller:master-2020-06-17-a9d9169"
	obs.ThanosReceiveController.Version = "master-2020-06-17-a9d9169"
	return obs
}

func newAPIRBAC() observatoriumv1alpha1.APIRBAC {
	return observatoriumv1alpha1.APIRBAC{
		Roles: []observatoriumv1alpha1.RBACRole{
			{
				Name: "read-write",
				Resources: []string{
					"metrics",
				},
				Permissions: []observatoriumv1alpha1.Permission{
					observatoriumv1alpha1.Write,
					observatoriumv1alpha1.Read,
				},
				Tenants: []string{
					config.GetDefaultTenantName(),
				},
			},
		},
		RoleBindings: []observatoriumv1alpha1.RBACRoleBinding{
			{
				Name: config.GetDefaultTenantName(),
				Roles: []string{
					"read-write",
				},
				Subjects: []observatoriumv1alpha1.Subject{
					{
						Name: "admin@example.com",
						Kind: observatoriumv1alpha1.User,
					},
				},
			},
		},
	}
}

func newAPITenants() []observatoriumv1alpha1.APITenant {
	return []observatoriumv1alpha1.APITenant{
		{
			Name: config.GetDefaultTenantName(),
			ID:   "1610b0c3-c509-4592-a256-a1871353dbfa",
			OIDC: observatoriumv1alpha1.TenantOIDC{
				ClientID:      "test",
				ClientSecret:  "ZXhhbXBsZS1hcHAtc2VjcmV0",
				IssuerURL:     "http://ec2-107-21-40-191.compute-1.amazonaws.com:5556/dex",
				UsernameClaim: "email",
			},
		},
	}
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

func updateObservatoriumSpec(
	c client.Client,
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {

	//TODO: update new values from CR to observatorium CR

	// Merge observatorium Spec with the default values and customized values
	//defaultSpec := newDefaultObservatoriumSpec()
	// runtimeSpec := mco.Spec.Observatorium
	// if !reflect.DeepEqual(defaultSpec, runtimeSpec) {
	// 	if err := mergo.MergeWithOverwrite(defaultSpec, runtimeSpec); err != nil {
	// 		return &reconcile.Result{}, err
	// 	}
	// 	mergeVolumeClaimTemplate(defaultSpec.Compact.VolumeClaimTemplate, runtimeSpec.Compact.VolumeClaimTemplate)
	// 	mergeVolumeClaimTemplate(defaultSpec.Rule.VolumeClaimTemplate, runtimeSpec.Rule.VolumeClaimTemplate)
	// 	mergeVolumeClaimTemplate(defaultSpec.Receivers.VolumeClaimTemplate, runtimeSpec.Receivers.VolumeClaimTemplate)
	// 	mergeVolumeClaimTemplate(defaultSpec.Store.VolumeClaimTemplate, runtimeSpec.Store.VolumeClaimTemplate)
	// 	mco.Spec.Observatorium = defaultSpec
	// }
	return nil, nil
}

func mergeVolumeClaimTemplate(oldVolumn,
	newVolumn observatoriumv1alpha1.VolumeClaimTemplate) observatoriumv1alpha1.VolumeClaimTemplate {
	requestRes := newVolumn.Spec.Resources.Requests
	limitRes := newVolumn.Spec.Resources.Limits
	if requestRes != nil {
		oldVolumn.Spec.Resources.Requests[v1.ResourceStorage] = requestRes[v1.ResourceStorage]
	}
	if limitRes != nil {
		oldVolumn.Spec.Resources.Limits[v1.ResourceStorage] = limitRes[v1.ResourceStorage]
	}
	return oldVolumn
}
