// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package multiclusterobservability

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	obsv1alpha1 "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/util"
)

const (
	obsAPIGateway = "observatorium-api"

	readOnlyRoleName  = "read-only-metrics"
	writeOnlyRoleName = "write-only-metrics"
)

// GenerateObservatoriumCR returns Observatorium cr defined in MultiClusterObservability
func GenerateObservatoriumCR(
	cl client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {

	labels := map[string]string{
		"app": mcoconfig.GetOperandName(mcoconfig.Observatorium),
	}

	storageClassSelected, err := getStorageClass(mco, cl)
	if err != nil {
		return &ctrl.Result{}, err
	}

	log.Info("storageClassSelected", "storageClassSelected", storageClassSelected)

	observatoriumCR := &obsv1alpha1.Observatorium{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoconfig.GetOperandName(mcoconfig.Observatorium),
			Namespace: mcoconfig.GetDefaultNamespace(),
			Labels:    labels,
		},
		Spec: *newDefaultObservatoriumSpec(mco, storageClassSelected),
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, observatoriumCR, scheme); err != nil {
		return &ctrl.Result{}, err
	}

	// Check if this Observatorium CR already exists
	observatoriumCRFound := &obsv1alpha1.Observatorium{}
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
			"observatorium", observatoriumCR.Name,
		)
		err = cl.Create(context.TODO(), observatoriumCR)
		if err != nil {
			return &ctrl.Result{}, err
		}
		return nil, nil
	} else if err != nil {
		return &ctrl.Result{}, err
	}

	oldSpec := observatoriumCRFound.Spec
	newSpec := observatoriumCR.Spec
	oldSpecBytes, _ := yaml.Marshal(oldSpec)
	newSpecBytes, _ := yaml.Marshal(newSpec)
	if bytes.Equal(newSpecBytes, oldSpecBytes) {
		return nil, nil
	}

	// keep the tenant id unchanged
	for i, newTenant := range newSpec.API.Tenants {
		for _, oldTenant := range oldSpec.API.Tenants {
			updateTenantID(&newSpec, newTenant, oldTenant, i)
		}
	}

	log.Info("Updating observatorium CR",
		"observatorium", observatoriumCR.Name,
	)

	newObj := observatoriumCRFound.DeepCopy()
	newObj.Spec = newSpec
	err = cl.Update(context.TODO(), newObj)
	if err != nil {
		log.Error(err, "Failed to update observatorium CR %s", observatoriumCR.Name)
		// add timeout for update failure avoid update conflict
		return &ctrl.Result{RequeueAfter: time.Second * 3}, err
	}

	// delete the store-share statefulset in scalein scenario
	err = deleteStoreSts(cl, observatoriumCR.Name,
		*oldSpec.Thanos.Store.Shards, *newSpec.Thanos.Store.Shards)
	if err != nil {
		return &ctrl.Result{}, err
	}

	return nil, nil
}

func updateTenantID(
	newSpec *obsv1alpha1.ObservatoriumSpec,
	newTenant obsv1alpha1.APITenant,
	oldTenant obsv1alpha1.APITenant,
	idx int) {

	if oldTenant.Name == newTenant.Name && newTenant.ID == oldTenant.ID {
		return
	}

	newSpec.API.Tenants[idx].ID = oldTenant.ID
	for j, hashring := range newSpec.Hashrings {
		if util.Contains(hashring.Tenants, newTenant.ID) {
			newSpec.Hashrings[j].Tenants = util.Remove(newSpec.Hashrings[j].Tenants, newTenant.ID)
			newSpec.Hashrings[j].Tenants = append(newSpec.Hashrings[0].Tenants, oldTenant.ID)
		}
	}
}

// GenerateAPIGatewayRoute defines aaa
func GenerateAPIGatewayRoute(
	runclient client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {

	apiGateway := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAPIGateway,
			Namespace: mcoconfig.GetDefaultNamespace(),
		},
		Spec: routev1.RouteSpec{
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString("public"),
			},
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: mcoconfig.GetOperandNamePrefix() + "observatorium-api",
			},
			TLS: &routev1.TLSConfig{
				Termination:                   routev1.TLSTerminationPassthrough,
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyNone,
			},
		},
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err := controllerutil.SetControllerReference(mco, apiGateway, scheme); err != nil {
		return &ctrl.Result{}, err
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
			return &ctrl.Result{}, err
		}
	}

	return nil, nil
}

func newDefaultObservatoriumSpec(mco *mcov1beta2.MultiClusterObservability,
	scSelected string) *obsv1alpha1.ObservatoriumSpec {

	obs := &obsv1alpha1.ObservatoriumSpec{}
	obs.SecurityContext = &v1.SecurityContext{}
	obs.PullSecret = mcoconfig.GetImagePullSecret(mco.Spec)
	obs.NodeSelector = mco.Spec.NodeSelector
	obs.Tolerations = mco.Spec.Tolerations
	obs.API = newAPISpec(mco)
	obs.Thanos = newThanosSpec(mco, scSelected)
	if util.ProxyEnvVarsAreSet() {
		obs.EnvVars = newEnvVars()
	}

	obs.Hashrings = []*obsv1alpha1.Hashring{
		{Hashring: "default", Tenants: []string{mcoconfig.GetTenantUID()}},
	}

	obs.ObjectStorageConfig.Thanos = &obsv1alpha1.ThanosObjectStorageConfigSpec{}
	if mco.Spec.StorageConfig != nil && mco.Spec.StorageConfig.MetricObjectStorage != nil {
		objStorageConf := mco.Spec.StorageConfig.MetricObjectStorage
		obs.ObjectStorageConfig.Thanos.Name = objStorageConf.Name
		obs.ObjectStorageConfig.Thanos.Key = objStorageConf.Key
	}
	return obs
}

// return proxy variables
// OLM set these environment variables as a unit
func newEnvVars() map[string]string {
	return map[string]string{
		"HTTP_PROXY":  os.Getenv("HTTP_PROXY"),
		"HTTPS_PROXY": os.Getenv("HTTPS_PROXY"),
		"NO_PROXY":    os.Getenv("NO_PROXY"),
	}
}

func newAPIRBAC() obsv1alpha1.APIRBAC {
	return obsv1alpha1.APIRBAC{
		Roles: []obsv1alpha1.RBACRole{
			{
				Name: readOnlyRoleName,
				Resources: []string{
					"metrics",
				},
				Permissions: []obsv1alpha1.Permission{
					obsv1alpha1.Read,
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
				Permissions: []obsv1alpha1.Permission{
					obsv1alpha1.Write,
				},
				Tenants: []string{
					mcoconfig.GetDefaultTenantName(),
				},
			},
		},
		RoleBindings: []obsv1alpha1.RBACRoleBinding{
			{
				Name: readOnlyRoleName,
				Roles: []string{
					readOnlyRoleName,
				},
				Subjects: []obsv1alpha1.Subject{
					{
						Name: config.GrafanaCN,
						Kind: obsv1alpha1.User,
					},
				},
			},
			{
				Name: writeOnlyRoleName,
				Roles: []string{
					writeOnlyRoleName,
				},
				Subjects: []obsv1alpha1.Subject{
					{
						Name: config.ManagedClusterOU,
						Kind: obsv1alpha1.Group,
					},
				},
			},
		},
	}
}

func newAPITenants() []obsv1alpha1.APITenant {
	return []obsv1alpha1.APITenant{
		{
			Name: mcoconfig.GetDefaultTenantName(),
			ID:   mcoconfig.GetTenantUID(),
			MTLS: &obsv1alpha1.TenantMTLS{
				SecretName: config.ClientCACerts,
				CAKey:      "tls.crt",
			},
		},
	}
}

func newAPITLS() obsv1alpha1.TLS {
	return obsv1alpha1.TLS{
		SecretName: config.ServerCerts,
		CertKey:    "tls.crt",
		KeyKey:     "tls.key",
		CAKey:      "ca.crt",
		ServerName: config.ServerCertCN,
	}
}

func newAPISpec(mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.APISpec {
	apiSpec := obsv1alpha1.APISpec{}
	apiSpec.RBAC = newAPIRBAC()
	apiSpec.Tenants = newAPITenants()
	apiSpec.TLS = newAPITLS()
	apiSpec.Replicas = mcoconfig.GetReplicas(mcoconfig.ObservatoriumAPI, mco.Spec.AdvancedConfig)
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		apiSpec.Resources = mcoconfig.GetResources(config.ObservatoriumAPI, mco.Spec.AdvancedConfig)
	}
	//set the default observatorium components' image
	apiSpec.Image = mcoconfig.DefaultImgRepository + "/" + mcoconfig.ObservatoriumAPIImgName +
		":" + mcoconfig.DefaultImgTagSuffix
	replace, image := mcoconfig.ReplaceImage(mco.Annotations, apiSpec.Image, mcoconfig.ObservatoriumAPIImgName)
	if replace {
		apiSpec.Image = image
	}
	apiSpec.ServiceMonitor = true
	return apiSpec
}

func newReceiversSpec(
	mco *mcov1beta2.MultiClusterObservability,
	scSelected string) obsv1alpha1.ReceiversSpec {
	receSpec := obsv1alpha1.ReceiversSpec{}
	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.RetentionConfig != nil &&
		mco.Spec.AdvancedConfig.RetentionConfig.RetentionInLocal != "" {
		receSpec.Retention = mco.Spec.AdvancedConfig.RetentionConfig.RetentionInLocal
	} else {
		receSpec.Retention = mcoconfig.RetentionInLocal
	}

	receSpec.Replicas = mcoconfig.GetReplicas(mcoconfig.ThanosReceive, mco.Spec.AdvancedConfig)
	if *receSpec.Replicas < 3 {
		receSpec.ReplicationFactor = receSpec.Replicas
	} else {
		receSpec.ReplicationFactor = &config.Replicas3
	}

	receSpec.ServiceMonitor = true
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		receSpec.Resources = mcoconfig.GetResources(config.ThanosReceive, mco.Spec.AdvancedConfig)
	}
	receSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.ReceiveStorageSize,
		scSelected)

	return receSpec
}

func newRuleSpec(mco *mcov1beta2.MultiClusterObservability, scSelected string) obsv1alpha1.RuleSpec {
	ruleSpec := obsv1alpha1.RuleSpec{}
	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.RetentionConfig != nil &&
		mco.Spec.AdvancedConfig.RetentionConfig.BlockDuration != "" {
		ruleSpec.BlockDuration = mco.Spec.AdvancedConfig.RetentionConfig.BlockDuration
	} else {
		ruleSpec.BlockDuration = mcoconfig.BlockDuration
	}
	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.RetentionConfig != nil &&
		mco.Spec.AdvancedConfig.RetentionConfig.RetentionInLocal != "" {
		ruleSpec.Retention = mco.Spec.AdvancedConfig.RetentionConfig.RetentionInLocal
	} else {
		ruleSpec.Retention = mcoconfig.RetentionInLocal
	}
	ruleSpec.Replicas = mcoconfig.GetReplicas(mcoconfig.ThanosRule, mco.Spec.AdvancedConfig)

	ruleSpec.ServiceMonitor = true
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		ruleSpec.Resources = mcoconfig.GetResources(config.ThanosRule, mco.Spec.AdvancedConfig)
		ruleSpec.ReloaderResources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ThanosRuleReloaderCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ThanosRuleReloaderMemoryRequets),
			},
		}
	}

	ruleSpec.ReloaderImage = mcoconfig.ConfigmapReloaderImgRepo + "/" +
		mcoconfig.ConfigmapReloaderImgName + ":" + mcoconfig.ConfigmapReloaderImgTagSuffix
	found, reloaderImage := mcoconfig.ReplaceImage(mco.Annotations,
		mcoconfig.ConfigmapReloaderImgRepo, mcoconfig.ConfigmapReloaderKey)
	if found {
		ruleSpec.ReloaderImage = reloaderImage
	}

	ruleSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.RuleStorageSize,
		scSelected)

	//configure alertmanager in ruler
	//ruleSpec.AlertmanagerURLs = []string{mcoconfig.AlertmanagerURL}
	ruleSpec.AlertmanagerConfigFile = obsv1alpha1.AlertmanagerConfigFile{
		Name: mcoconfig.AlertmanagersDefaultConfigMapName,
		Key:  mcoconfig.AlertmanagersDefaultConfigFileKey,
	}

	ruleSpec.ExtraVolumeMounts = []obsv1alpha1.VolumeMount{
		{
			Type:      obsv1alpha1.VolumeMountTypeConfigMap,
			MountPath: mcoconfig.AlertmanagersDefaultCaBundleMountPath,
			Name:      mcoconfig.AlertmanagersDefaultCaBundleName,
			Key:       mcoconfig.AlertmanagersDefaultCaBundleKey,
		},
	}

	ruleSpec.RulesConfig = []obsv1alpha1.RuleConfig{
		{
			Name: mcoconfig.AlertRuleDefaultConfigMapName,
			Key:  mcoconfig.AlertRuleDefaultFileKey,
		},
	}

	if mcoconfig.HasCustomRuleConfigMap() {
		customRuleConfig := []obsv1alpha1.RuleConfig{
			{
				Name: mcoconfig.AlertRuleCustomConfigMapName,
				Key:  mcoconfig.AlertRuleCustomFileKey,
			},
		}
		ruleSpec.RulesConfig = append(ruleSpec.RulesConfig, customRuleConfig...)
	} else {
		ruleSpec.RulesConfig = []obsv1alpha1.RuleConfig{
			{
				Name: mcoconfig.AlertRuleDefaultConfigMapName,
				Key:  mcoconfig.AlertRuleDefaultFileKey,
			},
		}
	}

	return ruleSpec
}

func newStoreSpec(mco *mcov1beta2.MultiClusterObservability, scSelected string) obsv1alpha1.StoreSpec {
	storeSpec := obsv1alpha1.StoreSpec{}
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		storeSpec.Resources = mcoconfig.GetResources(config.ThanosStoreShard, mco.Spec.AdvancedConfig)
	}

	storeSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.StoreStorageSize,
		scSelected)

	storeSpec.Shards = mcoconfig.GetReplicas(mcoconfig.ThanosStoreShard, mco.Spec.AdvancedConfig)
	storeSpec.ServiceMonitor = true
	storeSpec.Cache = newMemCacheSpec(mcoconfig.ThanosStoreMemcached, mco)

	return storeSpec
}

func newMemCacheSpec(component string, mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.MemCacheSpec {
	var cacheConfig *mcov1beta2.CacheConfig
	if mco.Spec.AdvancedConfig != nil {
		if component == mcoconfig.ThanosStoreMemcached {
			cacheConfig = mco.Spec.AdvancedConfig.StoreMemcached
		} else {
			cacheConfig = mco.Spec.AdvancedConfig.QueryFrontendMemcached
		}
	}
	memCacheSpec := obsv1alpha1.MemCacheSpec{}
	memCacheSpec.Image = mcoconfig.MemcachedImgRepo + "/" +
		mcoconfig.MemcachedImgName + ":" + mcoconfig.MemcachedImgTag
	memCacheSpec.Version = mcoconfig.MemcachedImgTag
	memCacheSpec.Replicas = mcoconfig.GetReplicas(component, mco.Spec.AdvancedConfig)

	memCacheSpec.ServiceMonitor = true
	memCacheSpec.ExporterImage = mcoconfig.MemcachedExporterImgRepo + "/" +
		mcoconfig.MemcachedExporterImgName + ":" + mcoconfig.MemcachedExporterImgTag
	memCacheSpec.ExporterVersion = mcoconfig.MemcachedExporterImgTag
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		memCacheSpec.Resources = mcoconfig.GetResources(component, mco.Spec.AdvancedConfig)
		memCacheSpec.ExporterResources = mcoconfig.GetResources(mcoconfig.MemcachedExporter, mco.Spec.AdvancedConfig)
	}

	found, image := mcoconfig.ReplaceImage(mco.Annotations, memCacheSpec.Image, mcoconfig.MemcachedImgName)
	if found {
		memCacheSpec.Image = image
	}

	found, image = mcoconfig.ReplaceImage(mco.Annotations, memCacheSpec.ExporterImage, mcoconfig.MemcachedExporterKey)
	if found {
		memCacheSpec.ExporterImage = image
	}
	if cacheConfig != nil && cacheConfig.MemoryLimitMB != nil {
		memCacheSpec.MemoryLimitMB = cacheConfig.MemoryLimitMB
	} else {
		memCacheSpec.MemoryLimitMB = &mcoconfig.MemoryLimitMB
	}
	if cacheConfig != nil && cacheConfig.ConnectionLimit != nil {
		memCacheSpec.ConnectionLimit = cacheConfig.ConnectionLimit
	} else {
		memCacheSpec.ConnectionLimit = &mcoconfig.ConnectionLimit
	}
	if cacheConfig != nil && cacheConfig.MaxItemSize != "" {
		memCacheSpec.MaxItemSize = cacheConfig.MaxItemSize
	} else {
		memCacheSpec.MaxItemSize = mcoconfig.MaxItemSize
	}

	return memCacheSpec
}

func newThanosSpec(mco *mcov1beta2.MultiClusterObservability, scSelected string) obsv1alpha1.ThanosSpec {
	thanosSpec := obsv1alpha1.ThanosSpec{}
	thanosSpec.Image = mcoconfig.DefaultImgRepository + "/" + mcoconfig.ThanosImgName +
		":" + mcoconfig.DefaultImgTagSuffix

	thanosSpec.Compact = newCompactSpec(mco, scSelected)
	thanosSpec.Receivers = newReceiversSpec(mco, scSelected)
	thanosSpec.Rule = newRuleSpec(mco, scSelected)
	thanosSpec.Store = newStoreSpec(mco, scSelected)
	thanosSpec.ReceiveController = newReceiverControllerSpec(mco)
	thanosSpec.Query = newQuerySpec(mco)
	thanosSpec.QueryFrontend = newQueryFrontendSpec(mco)

	replace, image := mcoconfig.ReplaceImage(mco.Annotations, thanosSpec.Image, mcoconfig.ThanosImgName)
	if replace {
		thanosSpec.Image = image
	}
	return thanosSpec
}

func newQueryFrontendSpec(mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.QueryFrontendSpec {
	queryFrontendSpec := obsv1alpha1.QueryFrontendSpec{}
	queryFrontendSpec.Replicas = mcoconfig.GetReplicas(mcoconfig.ThanosQueryFrontend, mco.Spec.AdvancedConfig)
	queryFrontendSpec.ServiceMonitor = true
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		queryFrontendSpec.Resources = mcoconfig.GetResources(config.ThanosQueryFrontend, mco.Spec.AdvancedConfig)
	}
	queryFrontendSpec.Cache = newMemCacheSpec(mcoconfig.ThanosQueryFrontendMemcached, mco)
	return queryFrontendSpec
}

func newQuerySpec(mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.QuerySpec {
	querySpec := obsv1alpha1.QuerySpec{}
	querySpec.Replicas = mcoconfig.GetReplicas(mcoconfig.ThanosQuery, mco.Spec.AdvancedConfig)
	querySpec.ServiceMonitor = true
	querySpec.LookbackDelta = fmt.Sprintf("%ds", mco.Spec.ObservabilityAddonSpec.Interval*2)
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		querySpec.Resources = mcoconfig.GetResources(config.ThanosQuery, mco.Spec.AdvancedConfig)
	}
	return querySpec
}

func newReceiverControllerSpec(mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.ReceiveControllerSpec {
	receiveControllerSpec := obsv1alpha1.ReceiveControllerSpec{}
	receiveControllerSpec.Image = mcoconfig.ObservatoriumImgRepo + "/" +
		mcoconfig.ThanosReceiveControllerImgName +
		":" + mcoconfig.ThanosReceiveControllerImgTag
	receiveControllerSpec.ServiceMonitor = true
	receiveControllerSpec.Version = mcoconfig.ThanosReceiveControllerImgTag
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		receiveControllerSpec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ObservatoriumReceiveControllerCPURequets),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ObservatoriumReceiveControllerMemoryRequets),
			},
		}
	}
	replace, image := mcoconfig.ReplaceImage(mco.Annotations, receiveControllerSpec.Image,
		mcoconfig.ThanosReceiveControllerKey)
	if replace {
		receiveControllerSpec.Image = image
	}
	return receiveControllerSpec
}

func newCompactSpec(mco *mcov1beta2.MultiClusterObservability, scSelected string) obsv1alpha1.CompactSpec {
	compactSpec := obsv1alpha1.CompactSpec{}
	//Compactor, generally, does not need to be highly available.
	//Compactions are needed from time to time, only when new blocks appear.
	compactSpec.Replicas = &mcoconfig.Replicas1
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		compactSpec.Resources = mcoconfig.GetResources(config.ThanosCompact, mco.Spec.AdvancedConfig)
	}
	compactSpec.ServiceMonitor = true
	compactSpec.EnableDownsampling = mco.Spec.EnableDownsampling
	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.RetentionConfig != nil &&
		mco.Spec.AdvancedConfig.RetentionConfig.DeleteDelay != "" {
		compactSpec.DeleteDelay = mco.Spec.AdvancedConfig.RetentionConfig.DeleteDelay
	} else {
		compactSpec.DeleteDelay = mcoconfig.DeleteDelay
	}

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.RetentionConfig != nil &&
		mco.Spec.AdvancedConfig.RetentionConfig.RetentionResolutionRaw != "" {
		compactSpec.RetentionResolutionRaw = mco.Spec.AdvancedConfig.RetentionConfig.RetentionResolutionRaw
	} else {
		compactSpec.RetentionResolutionRaw = mcoconfig.RetentionResolutionRaw
	}

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.RetentionConfig != nil &&
		mco.Spec.AdvancedConfig.RetentionConfig.RetentionResolution5m != "" {
		compactSpec.RetentionResolution5m = mco.Spec.AdvancedConfig.RetentionConfig.RetentionResolution5m
	} else {
		compactSpec.RetentionResolution5m = mcoconfig.RetentionResolution5m
	}

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.RetentionConfig != nil &&
		mco.Spec.AdvancedConfig.RetentionConfig.RetentionResolution1h != "" {
		compactSpec.RetentionResolution1h = mco.Spec.AdvancedConfig.RetentionConfig.RetentionResolution1h
	} else {
		compactSpec.RetentionResolution1h = mcoconfig.RetentionResolution1h
	}

	compactSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.CompactStorageSize,
		scSelected)

	return compactSpec
}

func newVolumeClaimTemplate(size string, storageClass string) obsv1alpha1.VolumeClaimTemplate {
	vct := obsv1alpha1.VolumeClaimTemplate{}
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
	newVolumn obsv1alpha1.VolumeClaimTemplate) obsv1alpha1.VolumeClaimTemplate {
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

func deleteStoreSts(cl client.Client, name string, oldNum int32, newNum int32) error {
	if oldNum > newNum {
		for i := newNum; i < oldNum; i++ {
			stsName := fmt.Sprintf("%s-thanos-store-shard-%d", name, i)
			found := &appsv1.StatefulSet{}
			err := cl.Get(context.TODO(), types.NamespacedName{Name: stsName, Namespace: mcoconfig.GetDefaultNamespace()}, found)
			if err != nil {
				if !errors.IsNotFound(err) {
					log.Error(err, "Failed to get statefulset", "name", stsName)
					return err
				}
			} else {
				err = cl.Delete(context.TODO(), found)
				if err != nil {
					log.Error(err, "Failed to delete statefulset", "name", stsName)
					return err
				}
			}
		}
	}
	return nil
}
