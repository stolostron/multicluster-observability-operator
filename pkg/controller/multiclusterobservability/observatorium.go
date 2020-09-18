// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"bytes"
	"context"

	observatoriumv1alpha1 "github.com/observatorium/operator/api/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	storv1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	obsPartoOfName = "-observatorium"
	obsAPIGateway  = "observatorium-api"

	defaultThanosImage   = "quay.io/thanos/thanos:master-2020-08-12-70f89d83"
	defaultThanosVersion = "master-2020-08-12-70f89d83"

	thanosImgName = "thanos"

	readOnlyRoleName  = "read-only-metrics"
	writeOnlyRoleName = "write-only-metrics"
)

// GenerateObservatoriumCR returns Observatorium cr defined in MultiClusterObservability
func GenerateObservatoriumCR(
	cl client.Client, scheme *runtime.Scheme,
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {

	labels := map[string]string{
		"app": mco.Name,
	}

	storageClassSelected := mco.Spec.StorageConfig.StatefulSetStorageClass
	// for the test, the reader is just nil
	storageClassList := &storv1.StorageClassList{}
	err := cl.List(context.TODO(), storageClassList, &client.ListOptions{})
	if err != nil {
		return &reconcile.Result{}, err
	}
	hasValidSC := false
	storageClassDefault := ""
	for _, storageClass := range storageClassList.Items {
		if storageClass.ObjectMeta.Name == storageClassSelected {
			hasValidSC = true
			break
		}
		if storageClass.ObjectMeta.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			storageClassDefault = storageClass.ObjectMeta.Name
		}
	}
	if !hasValidSC {
		storageClassSelected = storageClassDefault
	}

	log.Info("storageClassSelected", "storageClassSelected", storageClassSelected)
	log.Info("storageClassDefault", "storageClassDefault", storageClassDefault)

	observatoriumCR := &observatoriumv1alpha1.Observatorium{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mco.Name + obsPartoOfName,
			Namespace: mco.Namespace,
			Labels:    labels,
		},
		Spec: *newDefaultObservatoriumSpec(mco, storageClassSelected),
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, observatoriumCR, scheme); err != nil {
		return &reconcile.Result{}, err
	}

	// Check if this Observatorium CR already exists
	observatoriumCRFound := &observatoriumv1alpha1.Observatorium{}
	err = cl.Get(
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
		err = cl.Create(context.TODO(), observatoriumCR)
		if err != nil {
			return &reconcile.Result{}, err
		}
		return nil, nil
	} else if err != nil {
		return &reconcile.Result{}, err
	}

	oldSpec := observatoriumCRFound.Spec
	newSpec := observatoriumCR.Spec
	// @TODO: resolve design issue on whether enable/disable downsampling will affact retension period config
	oldSpecBytes, _ := yaml.Marshal(oldSpec)
	newSpecBytes, _ := yaml.Marshal(newSpec)

	if res := bytes.Compare(newSpecBytes, oldSpecBytes); res != 0 {
		newObj := observatoriumCRFound.DeepCopy()

		// keep the tenant id unchanged
		for i, newTenant := range newSpec.API.Tenants {
			for _, oldTenant := range oldSpec.API.Tenants {
				if oldTenant.Name == newTenant.Name && newTenant.ID != oldTenant.ID {
					newSpec.API.Tenants[i].ID = oldTenant.ID
					for j, hashring := range newSpec.Hashrings {
						if util.Contains(hashring.Tenants, newTenant.ID) {
							newSpec.Hashrings[j].Tenants = util.Remove(newSpec.Hashrings[j].Tenants, newTenant.ID)
							newSpec.Hashrings[j].Tenants = append(newSpec.Hashrings[0].Tenants, oldTenant.ID)
						}
					}
				}
			}
		}

		newObj.Spec = newSpec
		err = cl.Update(context.TODO(), newObj)
		if err != nil {
			return &reconcile.Result{}, err
		}
	}

	return nil, nil
}

// GenerateAPIGatewayRoute defines aaa
func GenerateAPIGatewayRoute(
	runclient client.Client, scheme *runtime.Scheme,
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {

	apiGateway := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAPIGateway,
			Namespace: mco.Namespace,
		},
		Spec: routev1.RouteSpec{
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString("public"),
			},
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: mco.Name + "-observatorium-observatorium-api",
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationPassthrough,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyNone,
			},
		},
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, apiGateway, scheme); err != nil {
		return &reconcile.Result{}, err
	}

	err := runclient.Get(
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

	return nil, nil
}

func newDefaultObservatoriumSpec(mco *mcov1beta1.MultiClusterObservability,
	scSelected string) *observatoriumv1alpha1.ObservatoriumSpec {

	obs := &observatoriumv1alpha1.ObservatoriumSpec{}
	obs.API.RBAC = newAPIRBAC()
	obs.API.Tenants = newAPITenants()
	obs.API.TLS = newAPITLS()
	obs.API.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "Deployments")
	obs.Compact = newCompactSpec(mco, scSelected)

	obs.Hashrings = []*observatoriumv1alpha1.Hashring{
		{Hashring: "default", Tenants: []string{mcoconfig.GetTenantUID()}},
	}

	obs.ObjectStorageConfig.Thanos = &observatoriumv1alpha1.ThanosObjectStorageConfigSpec{}
	if mco.Spec.StorageConfig != nil && mco.Spec.StorageConfig.MetricObjectStorage != nil {
		objStorageConf := mco.Spec.StorageConfig.MetricObjectStorage
		obs.ObjectStorageConfig.Thanos.Name = objStorageConf.Name
		obs.ObjectStorageConfig.Thanos.Key = objStorageConf.Key
	}

	obs.Receivers = newReceiversSpec(mco, scSelected)
	obs.Rule = newRuleSpec(mco, scSelected)
	obs.Store = newStoreSpec(mco, scSelected)

	//set the default observatorium components' image
	obs.API.Image = "quay.io/observatorium/observatorium:latest"
	obs.API.Version = "latest"

	obs.ThanosReceiveController.Image = "quay.io/observatorium/thanos-receive-controller:master-2020-06-17-a9d9169"
	obs.ThanosReceiveController.Version = "master-2020-06-17-a9d9169"

	obs.Query.Image = defaultThanosImage
	obs.Query.Version = defaultThanosVersion
	obs.Query.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "Deployments")

	obs.QueryFrontend.Image = defaultThanosImage
	obs.QueryFrontend.Version = defaultThanosVersion
	obs.QueryFrontend.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "Deployments")

	if mcoconfig.IsNeededReplacement(mco.Annotations, obs.API.Image) {

		obs.API.Image = mcoconfig.GetAnnotationImageInfo().ImageRepository + "/observatorium:" +
			mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
		obs.API.Version = mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
	}
	if mcoconfig.IsNeededReplacement(mco.Annotations, obs.QueryFrontend.Image) {
		obs.QueryFrontend.Image = mcoconfig.GetAnnotationImageInfo().ImageRepository + "/cortex:" +
			mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
		obs.QueryFrontend.Version = mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
	}
	if mcoconfig.IsNeededReplacement(mco.Annotations, obs.ThanosReceiveController.Image) {
		obs.ThanosReceiveController.Image = mcoconfig.GetAnnotationImageInfo().ImageRepository +
			"/thanos-receive-controller:" + mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
		obs.ThanosReceiveController.Version = mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
	}
	if mcoconfig.IsNeededReplacement(mco.Annotations, obs.Query.Image) {
		obs.Query.Image = mcoconfig.GetAnnotationImageInfo().ImageRepository + "/" + thanosImgName + ":" +
			mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
		obs.Query.Version = mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
	}
	return obs
}

func newAPIRBAC() observatoriumv1alpha1.APIRBAC {
	return observatoriumv1alpha1.APIRBAC{
		Roles: []observatoriumv1alpha1.RBACRole{
			{
				Name: readOnlyRoleName,
				Resources: []string{
					"metrics",
				},
				Permissions: []observatoriumv1alpha1.Permission{
					observatoriumv1alpha1.Read,
				},
				Tenants: []string{
					mcoconfig.GetDefaultTenantName(),
				},
			},
			{
				Name: writeOnlyRoleName,
				Resources: []string{
					"metrics",
				},
				Permissions: []observatoriumv1alpha1.Permission{
					observatoriumv1alpha1.Write,
				},
				Tenants: []string{
					mcoconfig.GetDefaultTenantName(),
				},
			},
		},
		RoleBindings: []observatoriumv1alpha1.RBACRoleBinding{
			{
				Name: readOnlyRoleName,
				Roles: []string{
					readOnlyRoleName,
				},
				Subjects: []observatoriumv1alpha1.Subject{
					{
						Name: GetGrafanaSubject(),
						Kind: observatoriumv1alpha1.User,
					},
				},
			},
			{
				Name: writeOnlyRoleName,
				Roles: []string{
					writeOnlyRoleName,
				},
				Subjects: []observatoriumv1alpha1.Subject{
					{
						Name: GetManagedClusterOrg(),
						Kind: observatoriumv1alpha1.Group,
					},
				},
			},
		},
	}
}

func newAPITenants() []observatoriumv1alpha1.APITenant {
	return []observatoriumv1alpha1.APITenant{
		{
			Name: mcoconfig.GetDefaultTenantName(),
			ID:   mcoconfig.GetTenantUID(),
			MTLS: &observatoriumv1alpha1.TenantMTLS{
				SecretName: GetClientCACert(),
				CAKey:      "ca.crt",
			},
		},
	}
}

func newAPITLS() observatoriumv1alpha1.TLS {
	return observatoriumv1alpha1.TLS{
		SecretName: GetServerCerts(),
		CertKey:    "tls.crt",
		KeyKey:     "tls.key",
		CAKey:      "ca.crt",
		ServerName: serverCertificate,
	}
}

func newReceiversSpec(
	mco *mcov1beta1.MultiClusterObservability,
	scSelected string) observatoriumv1alpha1.ReceiversSpec {
	receSpec := observatoriumv1alpha1.ReceiversSpec{}
	receSpec.Image = defaultThanosImage
	receSpec.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "StatefulSet")
	receSpec.Version = defaultThanosVersion
	if mcoconfig.IsNeededReplacement(mco.Annotations, receSpec.Image) {
		receSpec.Image = mcoconfig.GetAnnotationImageInfo().ImageRepository + "/" +
			thanosImgName + ":" + mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
		receSpec.Version = mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
	}
	receSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.StatefulSetSize,
		scSelected)

	return receSpec
}

func newRuleSpec(mco *mcov1beta1.MultiClusterObservability, scSelected string) observatoriumv1alpha1.RuleSpec {
	ruleSpec := observatoriumv1alpha1.RuleSpec{}
	ruleSpec.Image = defaultThanosImage
	ruleSpec.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "StatefulSet")
	ruleSpec.Version = defaultThanosVersion
	if mcoconfig.IsNeededReplacement(mco.Annotations, ruleSpec.Image) {
		ruleSpec.Image = mcoconfig.GetAnnotationImageInfo().ImageRepository + "/" +
			thanosImgName + ":" + mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
		ruleSpec.Version = mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
	}
	ruleSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.StatefulSetSize,
		scSelected)

	return ruleSpec
}

func newStoreSpec(mco *mcov1beta1.MultiClusterObservability, scSelected string) observatoriumv1alpha1.StoreSpec {
	storeSpec := observatoriumv1alpha1.StoreSpec{}
	storeSpec.Image = defaultThanosImage
	storeSpec.Version = defaultThanosVersion
	if mcoconfig.IsNeededReplacement(mco.Annotations, storeSpec.Image) {
		storeSpec.Image = mcoconfig.GetAnnotationImageInfo().ImageRepository + "/" +
			thanosImgName + ":" + mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
		storeSpec.Version = mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
	}
	storeSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.StatefulSetSize,
		scSelected)
	storeSpec.Shards = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "StatefulSet")
	storeSpec.Cache = newStoreCacheSpec(mco)

	return storeSpec
}

func newStoreCacheSpec(mco *mcov1beta1.MultiClusterObservability) observatoriumv1alpha1.StoreCacheSpec {
	storeCacheSpec := observatoriumv1alpha1.StoreCacheSpec{}
	storeCacheSpec.Image = "docker.io/memcached:1.6.3-alpine"
	storeCacheSpec.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "StatefulSet")
	storeCacheSpec.Version = "1.6.3-alpine"
	storeCacheSpec.ExporterImage = "prom/memcached-exporter:v0.6.0"
	storeCacheSpec.ExporterVersion = "v0.6.0"

	if mcoconfig.IsNeededReplacement(mco.Annotations, storeCacheSpec.Image) {
		storeCacheSpec.Image = mcoconfig.GetAnnotationImageInfo().ImageRepository + "/memcached:" +
			mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
		storeCacheSpec.Version = mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
	}
	if mcoconfig.IsNeededReplacement(mco.Annotations, storeCacheSpec.ExporterImage) {
		storeCacheSpec.ExporterImage = mcoconfig.GetAnnotationImageInfo().ImageRepository + "/memcached-exporter:" +
			mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
		storeCacheSpec.ExporterVersion = mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
	}

	limit := int32(1024)
	storeCacheSpec.MemoryLimitMB = &limit

	return storeCacheSpec
}

func newCompactSpec(mco *mcov1beta1.MultiClusterObservability, scSelected string) observatoriumv1alpha1.CompactSpec {
	compactSpec := observatoriumv1alpha1.CompactSpec{}
	compactSpec.Image = defaultThanosImage
	compactSpec.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "StatefulSet")
	compactSpec.Version = defaultThanosVersion
	if mcoconfig.IsNeededReplacement(mco.Annotations, compactSpec.Image) {
		compactSpec.Image = mcoconfig.GetAnnotationImageInfo().ImageRepository + "/" + thanosImgName + ":" +
			mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
		compactSpec.Version = mcoconfig.GetAnnotationImageInfo().ImageTagSuffix
	}
	compactSpec.EnableDownsampling = mco.Spec.EnableDownSampling
	compactSpec.RetentionResolutionRaw = mco.Spec.RetentionResolutionRaw
	compactSpec.RetentionResolution5m = mco.Spec.RetentionResolution5m
	compactSpec.RetentionResolution1h = mco.Spec.RetentionResolution1h
	compactSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.StatefulSetSize,
		scSelected)

	return compactSpec
}

func newVolumeClaimTemplate(size string, storageClass string) observatoriumv1alpha1.VolumeClaimTemplate {
	vct := observatoriumv1alpha1.VolumeClaimTemplate{}
	vct.Spec = v1.PersistentVolumeClaimSpec{
		AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
		StorageClassName: &storageClass,
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse(size),
			},
		},
	}
	return vct
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
