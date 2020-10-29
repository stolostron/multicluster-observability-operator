// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"bytes"
	"context"

	observatoriumv1alpha1 "github.com/observatorium/operator/api/v1alpha1"
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
	"sigs.k8s.io/yaml"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	mcoconfig "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	obsPartoOfName = "-observatorium"
	obsAPIGateway  = "observatorium-api"

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

	storageClassSelected, err := getStorageClass(mco, cl)
	if err != nil {
		return &reconcile.Result{}, err
	}

	log.Info("storageClassSelected", "storageClassSelected", storageClassSelected)

	observatoriumCR := &observatoriumv1alpha1.Observatorium{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mco.Name + obsPartoOfName,
			Namespace: config.GetDefaultNamespace(),
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
			Namespace: config.GetDefaultNamespace(),
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
	obs.API.Image = mcoconfig.ObservatoriumImgRepo + "/" + mcoconfig.ObservatoriumAPIImgName +
		":" + mcoconfig.ObservatoriumAPIImgTag
	obs.API.Version = mcoconfig.ObservatoriumAPIImgTag

	obs.ThanosReceiveController.Image = mcoconfig.ObservatoriumImgRepo + "/" +
		mcoconfig.ThanosReceiveControllerImgName +
		":" + mcoconfig.ThanosReceiveControllerImgTag
	obs.ThanosReceiveController.Version = mcoconfig.ThanosReceiveControllerImgTag

	obs.Query.Image = mcoconfig.ThanosImgRepo + "/" + mcoconfig.ThanosImgName + ":" + mcoconfig.ThanosImgTag
	obs.Query.Version = mcoconfig.ThanosImgTag
	obs.Query.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "Deployments")

	obs.QueryFrontend.Image = mcoconfig.ThanosImgRepo + "/" + mcoconfig.ThanosImgName + ":" + mcoconfig.ThanosImgTag
	obs.QueryFrontend.Version = mcoconfig.ThanosImgTag
	obs.QueryFrontend.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "Deployments")

	replace, image := mcoconfig.ReplaceImage(mco.Annotations, obs.API.Image, mcoconfig.ObservatoriumAPIImgName)
	if replace {
		obs.API.Image = image
	}
	replace, image = mcoconfig.ReplaceImage(mco.Annotations, obs.QueryFrontend.Image, mcoconfig.ThanosImgName)
	if replace {
		obs.QueryFrontend.Image = image
	}
	replace, image = mcoconfig.ReplaceImage(mco.Annotations, obs.Query.Image, mcoconfig.ThanosImgName)
	if replace {
		obs.Query.Image = image
	}
	replace, image = mcoconfig.ReplaceImage(mco.Annotations, obs.ThanosReceiveController.Image,
		mcoconfig.ThanosReceiveControllerKey)
	if replace {
		obs.ThanosReceiveController.Image = image
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
	receSpec.Image = mcoconfig.ThanosImgRepo + "/" + mcoconfig.ThanosImgName + ":" + mcoconfig.ThanosImgTag
	receSpec.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "StatefulSet")
	receSpec.Version = mcoconfig.ThanosImgTag
	found, image := mcoconfig.ReplaceImage(mco.Annotations, receSpec.Image, mcoconfig.ThanosImgName)
	if found {
		receSpec.Image = image
	}
	receSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.StatefulSetSize,
		scSelected)

	return receSpec
}

func newRuleSpec(mco *mcov1beta1.MultiClusterObservability, scSelected string) observatoriumv1alpha1.RuleSpec {
	ruleSpec := observatoriumv1alpha1.RuleSpec{}
	ruleSpec.Image = mcoconfig.ThanosImgRepo + "/" + mcoconfig.ThanosImgName + ":" + mcoconfig.ThanosImgTag
	ruleSpec.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "StatefulSet")
	ruleSpec.Version = mcoconfig.ThanosImgTag
	found, image := mcoconfig.ReplaceImage(mco.Annotations, ruleSpec.Image, mcoconfig.ThanosImgName)
	if found {
		ruleSpec.Image = image
	}
	ruleSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.StatefulSetSize,
		scSelected)

	//configure alertmanager in ruler
	ruleSpec.AlertmanagersURL = []string{mcoconfig.AlertmanagerURL}
	ruleSpec.RulesConfig = []observatoriumv1alpha1.RuleConfig{
		{
			Name: mcoconfig.AlertRuleDefaultConfigMapName,
			Key:  mcoconfig.AlertRuleDefaultFileKey,
		},
	}

	if mcoconfig.HasCustomRuleConfigMap() {
		customRuleConfig := []observatoriumv1alpha1.RuleConfig{
			{
				Name: mcoconfig.AlertRuleCustomConfigMapName,
				Key:  mcoconfig.AlertRuleCustomFileKey,
			},
		}
		ruleSpec.RulesConfig = append(ruleSpec.RulesConfig, customRuleConfig...)
	} else {
		ruleSpec.RulesConfig = []observatoriumv1alpha1.RuleConfig{
			{
				Name: mcoconfig.AlertRuleDefaultConfigMapName,
				Key:  mcoconfig.AlertRuleDefaultFileKey,
			},
		}
	}

	return ruleSpec
}

func newStoreSpec(mco *mcov1beta1.MultiClusterObservability, scSelected string) observatoriumv1alpha1.StoreSpec {
	storeSpec := observatoriumv1alpha1.StoreSpec{}
	storeSpec.Image = mcoconfig.ThanosImgRepo + "/" + mcoconfig.ThanosImgName + ":" + mcoconfig.ThanosImgTag
	storeSpec.Version = mcoconfig.ThanosImgTag
	found, image := mcoconfig.ReplaceImage(mco.Annotations, storeSpec.Image, mcoconfig.ThanosImgName)
	if found {
		storeSpec.Image = image
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
	storeCacheSpec.Image = mcoconfig.MemcachedImgRepo + "/" +
		mcoconfig.MemcachedImgName + ":" + mcoconfig.MemcachedImgTag
	storeCacheSpec.Version = mcoconfig.MemcachedImgTag
	storeCacheSpec.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "StatefulSet")
	storeCacheSpec.ExporterImage = mcoconfig.MemcachedExporterImgRepo + "/" +
		mcoconfig.MemcachedExporterImgName + ":" + mcoconfig.MemcachedExporterImgTag
	storeCacheSpec.ExporterVersion = mcoconfig.MemcachedExporterImgTag

	found, image := mcoconfig.ReplaceImage(mco.Annotations, storeCacheSpec.Image, mcoconfig.MemcachedImgName)
	if found {
		storeCacheSpec.Image = image
	}

	found, image = mcoconfig.ReplaceImage(mco.Annotations, storeCacheSpec.ExporterImage, mcoconfig.MemcachedExporterKey)
	if found {
		storeCacheSpec.ExporterImage = image
	}

	limit := int32(1024)
	storeCacheSpec.MemoryLimitMB = &limit

	return storeCacheSpec
}

func newCompactSpec(mco *mcov1beta1.MultiClusterObservability, scSelected string) observatoriumv1alpha1.CompactSpec {
	compactSpec := observatoriumv1alpha1.CompactSpec{}
	compactSpec.Image = mcoconfig.ThanosImgRepo + "/" + mcoconfig.ThanosImgName + ":" + mcoconfig.ThanosImgTag
	compactSpec.Replicas = util.GetReplicaCount(mco.Spec.AvailabilityConfig, "StatefulSet")
	compactSpec.Version = mcoconfig.ThanosImgTag
	found, image := mcoconfig.ReplaceImage(mco.Annotations, compactSpec.Image, mcoconfig.ThanosImgName)
	if found {
		compactSpec.Image = image
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
