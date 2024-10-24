// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"

	// The import of crypto/md5 below is not for cryptographic use. It is used to hash the contents of files to track
	// changes and thus it's not a security issue.
	// nolint:gosec
	"crypto/md5" // #nosec G401 G501
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"reflect"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	obsv1alpha1 "github.com/stolostron/observatorium-operator/api/v1alpha1"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoutil "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

const (
	endpointsConfigName = "observability-remotewrite-endpoints"
	endpointsKey        = "endpoints.yaml"

	obsAPIGateway           = "observatorium-api"
	obsApiGatewayTargetPort = "public"

	obsCRConfigHashLabelName = "config-hash"

	readOnlyRoleName  = "read-only-metrics"
	writeOnlyRoleName = "write-only-metrics"

	endpointsRestartLabel = "endpoints/time-restarted"
)

// Fetch contents of the secret: observability-observatorium-api.
// Fetch contents of the configmap: observability-observatorium-api.
// Concatenate all of the above and hash their contents.
// If any of the secrets or configmaps aren't found, an empty struct of the respective type is used for the hash.
func hashObservatoriumCRConfig(cl client.Client) (string, error) {
	configMapToQuery := metav1.ObjectMeta{
		Name: mcoconfig.GetOperandNamePrefix() + mcoconfig.ObservatoriumAPI, Namespace: mcoconfig.GetDefaultNamespace(),
	}

	// The usage of crypto/md5 below is not for cryptographic use. It is used to hash the contents of files to track
	// changes and thus it's not a security issue.
	// nolint:gosec
	hasher := md5.New() // #nosec G401 G501
	resultConfigMap := &v1.ConfigMap{}
	err := cl.Get(context.TODO(), types.NamespacedName{
		Name:      configMapToQuery.Name,
		Namespace: configMapToQuery.Namespace,
	}, resultConfigMap)
	if err != nil && !k8serrors.IsNotFound(err) {
		return "", err
	}
	configMapData, err := yaml.Marshal(resultConfigMap.Data)
	if err != nil {
		return "", err
	}
	hasher.Write(configMapData)

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// GenerateObservatoriumCR returns Observatorium cr defined in MultiClusterObservability
func GenerateObservatoriumCR(
	cl client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {

	hash, err := hashObservatoriumCRConfig(cl)
	if err != nil {
		return &ctrl.Result{}, err
	}

	labels := map[string]string{
		"app":                    mcoconfig.GetOperandName(mcoconfig.Observatorium),
		obsCRConfigHashLabelName: hash,
	}

	storageClassSelected, err := getStorageClass(mco, cl)
	if err != nil {
		return &ctrl.Result{}, err
	}

	// fetch TLS secret mount path from the object store secret
	tlsSecretMountPath, err := getTLSSecretMountPath(cl, mco.Spec.StorageConfig.MetricObjectStorage)
	if err != nil {
		return &ctrl.Result{}, err
	}

	log.Info("storageClassSelected", "storageClassSelected", storageClassSelected)

	obsSpec, err := newDefaultObservatoriumSpec(cl, mco, storageClassSelected, tlsSecretMountPath)
	if err != nil {
		return &ctrl.Result{}, err
	}

	observatoriumCR := &obsv1alpha1.Observatorium{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoconfig.GetOperandName(mcoconfig.Observatorium),
			Namespace: mcoconfig.GetDefaultNamespace(),
			Labels:    labels,
		},
		Spec: *obsSpec,
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

	if err != nil && k8serrors.IsNotFound(err) {
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

	// keep the tenant id unchanged and ensure the new spec has the same tenant ID as the old spec to prevent Observatorium
	// from updating
	for i, newTenant := range newSpec.API.Tenants {
		for _, oldTenant := range oldSpec.API.Tenants {
			updateTenantID(&newSpec, newTenant, oldTenant, i)
		}
	}

	if equality.Semantic.DeepDerivative(oldSpec, newSpec) &&
		labels[obsCRConfigHashLabelName] == observatoriumCRFound.Labels[obsCRConfigHashLabelName] {
		return nil, nil
	}

	log.Info("Updating observatorium CR",
		"observatorium", observatoriumCR.Name,
	)

	newObj := observatoriumCRFound.DeepCopy()
	newObj.Spec = newSpec
	newObj.Labels[obsCRConfigHashLabelName] = observatoriumCR.Labels[obsCRConfigHashLabelName]
	err = cl.Update(context.TODO(), newObj)
	if err != nil {
		log.Error(err, "Failed to update observatorium CR %s", "name", observatoriumCR.Name)
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

func getTLSSecretMountPath(client client.Client,
	objectStorage *oashared.PreConfiguredStorage) (string, error) {
	found := &v1.Secret{}
	err := client.Get(
		context.TODO(),
		types.NamespacedName{Name: objectStorage.Name, Namespace: mcoconfig.GetDefaultNamespace()},
		found,
	)
	if err != nil {
		// report the status if the object store is not defined in checkObjStorageStatus method
		// here just ignore
		if k8serrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	data, ok := found.Data[objectStorage.Key]
	if !ok {
		return "", errors.New("failed to found the object storage configuration key from secret")
	}

	var objectConfg mcoconfig.ObjectStorgeConf
	err = yaml.Unmarshal(data, &objectConfg)
	if err != nil {
		return "", err
	}

	caFile := objectConfg.Config.HTTPConfig.TLSConfig.CAFile
	if caFile == "" {
		return "", nil
	}
	return path.Dir(caFile), nil
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
		if slices.Contains(hashring.Tenants, newTenant.ID) {
			newSpec.Hashrings[j].Tenants = util.Remove(newSpec.Hashrings[j].Tenants, newTenant.ID)
			newSpec.Hashrings[j].Tenants = append(newSpec.Hashrings[0].Tenants, oldTenant.ID)
		}
	}
}

// GenerateAPIGatewayRoute defines aaa
func GenerateAPIGatewayRoute(
	ctx context.Context,
	runclient client.Client, scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability) (*ctrl.Result, error) {

	apiGateway := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAPIGateway,
			Namespace: mcoconfig.GetDefaultNamespace(),
		},
		Spec: routev1.RouteSpec{
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString(obsApiGatewayTargetPort),
			},
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: mcoconfig.GetOperandNamePrefix() + obsAPIGateway,
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

	found := &routev1.Route{}
	err := runclient.Get(ctx, types.NamespacedName{Namespace: apiGateway.Namespace, Name: apiGateway.Name}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		log.Info("Creating a new route to expose observatorium api",
			"apiGateway.Namespace", apiGateway.Namespace,
			"apiGateway.Name", apiGateway.Name,
		)
		err = runclient.Create(context.TODO(), apiGateway)
		if err != nil {
			return &ctrl.Result{}, err
		}
		return nil, nil
	}

	var needsUpdate bool
	if found.Spec.TLS != nil {
		if found.Spec.TLS.Termination != routev1.TLSTerminationPassthrough {
			needsUpdate = true
			found.Spec.TLS.Termination = routev1.TLSTerminationPassthrough
		}

		if found.Spec.TLS.InsecureEdgeTerminationPolicy != routev1.InsecureEdgeTerminationPolicyNone {
			needsUpdate = true
			found.Spec.TLS.InsecureEdgeTerminationPolicy = routev1.InsecureEdgeTerminationPolicyNone
		}
	}

	if found.Spec.Port != nil && found.Spec.Port.TargetPort.String() != obsApiGatewayTargetPort {
		needsUpdate = true
		found.Spec.Port.TargetPort = intstr.FromString(obsApiGatewayTargetPort)
	}

	if found.Spec.To.Name != mcoconfig.GetOperandNamePrefix()+obsAPIGateway {
		needsUpdate = true
		found.Spec.To.Name = mcoconfig.GetOperandNamePrefix() + obsAPIGateway
	}

	if needsUpdate {
		log.Info("Updating Route for observatorium api",
			"apiGateway.Namespace", apiGateway.Namespace,
			"apiGateway.Name", apiGateway.Name,
		)
		err = runclient.Update(context.TODO(), found)
		if err != nil {
			log.Error(err, "failed update Route for observatorium api gateway",
				"apiGateway.Name", apiGateway.Name)
			return &ctrl.Result{}, err
		}
	}

	return nil, nil
}

func newDefaultObservatoriumSpec(cl client.Client, mco *mcov1beta2.MultiClusterObservability, scSelected string, tlsSecretMountPath string) (*obsv1alpha1.ObservatoriumSpec, error) {
	obs := &obsv1alpha1.ObservatoriumSpec{}
	obs.SecurityContext = &v1.SecurityContext{}
	obs.PullSecret = mcoconfig.GetImagePullSecret(mco.Spec)
	obs.NodeSelector = mco.Spec.NodeSelector
	obs.Tolerations = mco.Spec.Tolerations
	obsApi, err := newAPISpec(cl, mco)
	if err != nil {
		return obs, err
	}
	obs.API = obsApi
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
		obs.ObjectStorageConfig.Thanos.TLSSecretName = objStorageConf.TLSSecretName

		// Prefer using TLSSecretMountPath from the objstore config, rather than fetched one from secret.
		obs.ObjectStorageConfig.Thanos.TLSSecretMountPath = tlsSecretMountPath
		if objStorageConf.TLSSecretMountPath != "" {
			obs.ObjectStorageConfig.Thanos.TLSSecretMountPath = objStorageConf.TLSSecretMountPath
		}

		obs.ObjectStorageConfig.Thanos.TLSSecretMountPath = objStorageConf.TLSSecretMountPath
		obs.ObjectStorageConfig.Thanos.ServiceAccountProjection =
			mco.Spec.StorageConfig.MetricObjectStorage.ServiceAccountProjection
	}
	return obs, nil
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
						Name: mcoconfig.GrafanaCN,
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
						Name: mcoconfig.ManagedClusterOU,
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
				SecretName: mcoconfig.ClientCACerts,
				CAKey:      "tls.crt",
			},
		},
	}
}

func newAPITLS() obsv1alpha1.TLS {
	return obsv1alpha1.TLS{
		SecretName: mcoconfig.ServerCerts,
		CertKey:    "tls.crt",
		KeyKey:     "tls.key",
		CAKey:      "ca.crt",
		ServerName: mcoconfig.ServerCertCN,
	}
}

func applyEndpointsSecret(c client.Client, eps []mcoutil.RemoteWriteEndpointWithSecret) error {
	epsYaml, err := yaml.Marshal(eps)
	if err != nil {
		return err
	}
	epsYamlMap := map[string][]byte{}
	epsYamlMap[endpointsKey] = epsYaml
	epsSecret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      endpointsConfigName,
			Namespace: mcoconfig.GetDefaultNamespace(),
		},
		Data: epsYamlMap,
	}
	found := &v1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: endpointsConfigName,
		Namespace: mcoconfig.GetDefaultNamespace()}, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err = c.Create(context.TODO(), epsSecret)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !reflect.DeepEqual(epsYamlMap, found.Data) {
			epsSecret.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
			err = c.Update(context.TODO(), epsSecret)
			if err != nil {
				return err
			}
			err = util.UpdateDeployLabel(c, mcoconfig.GetOperandName(mcoconfig.ObservatoriumAPI),
				mcoconfig.GetDefaultNamespace(), endpointsRestartLabel)
			if err != nil {
				return err
			}
		}
	}
	return nil

}

func newAPISpec(c client.Client, mco *mcov1beta2.MultiClusterObservability) (obsv1alpha1.APISpec, error) {
	apiSpec := obsv1alpha1.APISpec{}
	apiSpec.RBAC = newAPIRBAC()
	apiSpec.Tenants = newAPITenants()
	apiSpec.TLS = newAPITLS()
	apiSpec.Replicas = mcoconfig.GetReplicas(mcoconfig.ObservatoriumAPI, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		apiSpec.Resources = mcoconfig.GetResources(mcoconfig.ObservatoriumAPI, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	}
	// set the default observatorium components' image
	apiSpec.Image = mcoconfig.DefaultImgRepository + "/" + mcoconfig.ObservatoriumAPIImgName +
		":" + mcoconfig.DefaultImgTagSuffix
	replace, image := mcoconfig.ReplaceImage(mco.Annotations, apiSpec.Image, mcoconfig.ObservatoriumAPIImgName)
	if replace {
		apiSpec.Image = image
	}
	apiSpec.ImagePullPolicy = mcoconfig.GetImagePullPolicy(mco.Spec)
	apiSpec.ServiceMonitor = true
	if mco.Spec.StorageConfig.WriteStorage != nil {
		var eps []mcoutil.RemoteWriteEndpointWithSecret
		var mountSecrets []string
		for _, storageConfig := range mco.Spec.StorageConfig.WriteStorage {
			storageSecret := &v1.Secret{}
			err := c.Get(context.TODO(), types.NamespacedName{Name: storageConfig.Name,
				Namespace: mcoconfig.GetDefaultNamespace()}, storageSecret)
			if err != nil {
				log.Error(err, "Failed to get the secret", "name", storageConfig.Name)
				return apiSpec, err
			} else {
				// add backup label
				err = addBackupLabel(c, storageConfig.Name, storageSecret)
				if err != nil {
					return apiSpec, err
				}

				data, ok := storageSecret.Data[storageConfig.Key]
				if !ok {
					log.Error(err, "Invalid key in secret", "name", storageConfig.Name, "key", storageConfig.Key)
					return apiSpec, fmt.Errorf("invalid key %s in secret %s", storageConfig.Key, storageConfig.Name)
				}
				ep := &mcoutil.RemoteWriteEndpointWithSecret{}
				err = yaml.Unmarshal(data, ep)
				if err != nil {
					log.Error(err, "Failed to unmarshal data in secret", "name", storageConfig.Name)
					return apiSpec, err
				}

				err = ep.Validate()
				if err != nil {
					log.Error(err, "Failed to validate data in secret", "name", storageConfig.Name)
					return apiSpec, err
				}

				newEp := &mcoutil.RemoteWriteEndpointWithSecret{
					Name: storageConfig.Name,
					URL:  ep.URL,
				}
				if ep.HttpClientConfig != nil {
					newConfig, mountS := mcoutil.Transform(*ep.HttpClientConfig)

					// add backup label
					for _, s := range mountS {
						err = addBackupLabel(c, s, nil)
						if err != nil {
							return apiSpec, err
						}
					}

					mountSecrets = append(mountSecrets, mountS...)
					newEp.HttpClientConfig = newConfig
				}
				eps = append(eps, *newEp)
			}
		}

		err := applyEndpointsSecret(c, eps)
		if err != nil {
			return apiSpec, err
		}
		if len(eps) > 0 {
			apiSpec.AdditionalWriteEndpoints = &obsv1alpha1.EndpointsConfig{
				EndpointsConfigSecret: endpointsConfigName,
			}
			if len(mountSecrets) > 0 {
				apiSpec.AdditionalWriteEndpoints.MountSecrets = mountSecrets
				apiSpec.AdditionalWriteEndpoints.MountPath = mcoutil.MountPath
			}
		}
	}
	return apiSpec, nil
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

	receSpec.Replicas = mcoconfig.GetReplicas(mcoconfig.ThanosReceive, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	if *receSpec.Replicas < 3 {
		receSpec.ReplicationFactor = receSpec.Replicas
	} else {
		var replicas3 int32 = 3
		receSpec.ReplicationFactor = &replicas3
	}

	receSpec.ServiceMonitor = true
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		receSpec.Resources = mcoconfig.GetResources(mcoconfig.ThanosReceive, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	}
	receSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.ReceiveStorageSize,
		scSelected)

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.Receive != nil &&
		mco.Spec.AdvancedConfig.Receive.ServiceAccountAnnotations != nil {
		receSpec.ServiceAccountAnnotations = mco.Spec.AdvancedConfig.Receive.ServiceAccountAnnotations
	}

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.Receive != nil &&
		mco.Spec.AdvancedConfig.Receive.Containers != nil {
		receSpec.Containers = mco.Spec.AdvancedConfig.Receive.Containers
	}
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

	if mco.Spec.AdvancedConfig != nil &&
		mco.Spec.AdvancedConfig.Rule != nil &&
		len(mco.Spec.AdvancedConfig.Rule.EvalInterval) > 0 {
		ruleSpec.EvalInterval = mco.Spec.AdvancedConfig.Rule.EvalInterval
	} else {
		ruleSpec.EvalInterval = fmt.Sprintf("%ds", mco.Spec.ObservabilityAddonSpec.Interval)
	}
	ruleSpec.Replicas = mcoconfig.GetReplicas(mcoconfig.ThanosRule, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)

	ruleSpec.ServiceMonitor = true
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		ruleSpec.Resources = mcoconfig.GetResources(mcoconfig.ThanosRule, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
		if mco.Spec.InstanceSize == "" {
			mco.Spec.InstanceSize = mcoconfig.Default
		}
		ruleSpec.ReloaderResources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse(mcoconfig.ThanosRuleReloaderCPURequest[mco.Spec.InstanceSize]),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(mcoconfig.ThanosRuleReloaderMemoryRequest[mco.Spec.InstanceSize]),
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
	ruleSpec.ReloaderImagePullPolicy = mcoconfig.GetImagePullPolicy(mco.Spec)

	ruleSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.RuleStorageSize,
		scSelected)

	// configure alertmanager in ruler
	// ruleSpec.AlertmanagerURLs = []string{mcoconfig.AlertmanagerURL}
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

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.Rule != nil &&
		mco.Spec.AdvancedConfig.Rule.ServiceAccountAnnotations != nil {
		ruleSpec.ServiceAccountAnnotations = mco.Spec.AdvancedConfig.Rule.ServiceAccountAnnotations
	}

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.Rule != nil &&
		mco.Spec.AdvancedConfig.Rule.Containers != nil {
		ruleSpec.Containers = mco.Spec.AdvancedConfig.Rule.Containers
	}

	return ruleSpec
}

func newStoreSpec(mco *mcov1beta2.MultiClusterObservability, scSelected string) obsv1alpha1.StoreSpec {
	storeSpec := obsv1alpha1.StoreSpec{}
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		storeSpec.Resources = mcoconfig.GetResources(mcoconfig.ThanosStoreShard, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	}

	storeSpec.VolumeClaimTemplate = newVolumeClaimTemplate(
		mco.Spec.StorageConfig.StoreStorageSize,
		scSelected)

	storeSpec.Shards = mcoconfig.GetReplicas(mcoconfig.ThanosStoreShard, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	storeSpec.ServiceMonitor = true
	storeSpec.Cache = newMemCacheSpec(mcoconfig.ThanosStoreMemcached, mco)

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.Store != nil &&
		mco.Spec.AdvancedConfig.Store.ServiceAccountAnnotations != nil {
		storeSpec.ServiceAccountAnnotations = mco.Spec.AdvancedConfig.Store.ServiceAccountAnnotations
	}

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.Store != nil &&
		mco.Spec.AdvancedConfig.Store.Containers != nil {
		storeSpec.Containers = mco.Spec.AdvancedConfig.Store.Containers
	}

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
	memCacheSpec.Replicas = mcoconfig.GetReplicas(component, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)

	memCacheSpec.ServiceMonitor = true
	memCacheSpec.ExporterImage = mcoconfig.MemcachedExporterImgRepo + "/" +
		mcoconfig.MemcachedExporterImgName + ":" + mcoconfig.MemcachedExporterImgTag
	memCacheSpec.ExporterVersion = mcoconfig.MemcachedExporterImgTag
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		memCacheSpec.Resources = mcoconfig.GetResources(component, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
		memCacheSpec.ExporterResources = mcoconfig.GetResources(mcoconfig.MemcachedExporter, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	}

	found, image := mcoconfig.ReplaceImage(mco.Annotations, memCacheSpec.Image, mcoconfig.MemcachedImgName)
	if found {
		memCacheSpec.Image = image
	}
	memCacheSpec.ImagePullPolicy = mcoconfig.GetImagePullPolicy(mco.Spec)

	found, image = mcoconfig.ReplaceImage(mco.Annotations, memCacheSpec.ExporterImage, mcoconfig.MemcachedExporterKey)
	if found {
		memCacheSpec.ExporterImage = image
	}
	memCacheSpec.ExporterImagePullPolicy = mcoconfig.GetImagePullPolicy(mco.Spec)
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
	thanosSpec.ImagePullPolicy = mcoconfig.GetImagePullPolicy(mco.Spec)
	return thanosSpec
}

func newQueryFrontendSpec(mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.QueryFrontendSpec {
	queryFrontendSpec := obsv1alpha1.QueryFrontendSpec{}
	queryFrontendSpec.Replicas = mcoconfig.GetReplicas(mcoconfig.ThanosQueryFrontend, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	queryFrontendSpec.ServiceMonitor = true
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		queryFrontendSpec.Resources = mcoconfig.GetResources(mcoconfig.ThanosQueryFrontend, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	}
	queryFrontendSpec.Cache = newMemCacheSpec(mcoconfig.ThanosQueryFrontendMemcached, mco)

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.QueryFrontend != nil &&
		mco.Spec.AdvancedConfig.QueryFrontend.Containers != nil {
		queryFrontendSpec.Containers = mco.Spec.AdvancedConfig.QueryFrontend.Containers
	}

	return queryFrontendSpec
}

func newQuerySpec(mco *mcov1beta2.MultiClusterObservability) obsv1alpha1.QuerySpec {
	querySpec := obsv1alpha1.QuerySpec{}
	querySpec.Replicas = mcoconfig.GetReplicas(mcoconfig.ThanosQuery, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	querySpec.ServiceMonitor = true
	// only set lookback-delta when the scrape interval * 2 is larger than 5 minute,
	// otherwise default value(5m) will be used.
	if mco.Spec.ObservabilityAddonSpec.Interval*2 > 300 {
		querySpec.LookbackDelta = fmt.Sprintf("%ds", mco.Spec.ObservabilityAddonSpec.Interval*2)
	}
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		querySpec.Resources = mcoconfig.GetResources(mcoconfig.ThanosQuery, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	}
	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.Query != nil &&
		mco.Spec.AdvancedConfig.Query.ServiceAccountAnnotations != nil {
		querySpec.ServiceAccountAnnotations = mco.Spec.AdvancedConfig.Query.ServiceAccountAnnotations
	}
	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.Query != nil &&
		mco.Spec.AdvancedConfig.Query.Containers != nil {
		querySpec.Containers = mco.Spec.AdvancedConfig.Query.Containers
	}

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.Query != nil &&
		mco.Spec.AdvancedConfig.Query.UsePrometheusEngine {
		querySpec.UsePrometheusEngine = true
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
		if mco.Spec.InstanceSize == "" {
			mco.Spec.InstanceSize = mcoconfig.Default
		}
		receiveControllerSpec.Resources = v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU): resource.MustParse(
					mcoconfig.ObservatoriumReceiveControllerCPURequest[mco.Spec.InstanceSize],
				),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse(
					mcoconfig.ObservatoriumReceiveControllerMemoryRequest[mco.Spec.InstanceSize],
				),
			},
		}
	}
	replace, image := mcoconfig.ReplaceImage(mco.Annotations, receiveControllerSpec.Image,
		mcoconfig.ThanosReceiveControllerKey)
	if replace {
		receiveControllerSpec.Image = image
	}
	receiveControllerSpec.ImagePullPolicy = mcoconfig.GetImagePullPolicy(mco.Spec)
	return receiveControllerSpec
}

func newCompactSpec(mco *mcov1beta2.MultiClusterObservability, scSelected string) obsv1alpha1.CompactSpec {
	compactSpec := obsv1alpha1.CompactSpec{}
	// Compactor, generally, does not need to be highly available.
	// Compactions are needed from time to time, only when new blocks appear.
	var replicas1 int32 = 1
	compactSpec.Replicas = &replicas1
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		compactSpec.Resources = mcoconfig.GetResources(mcoconfig.ThanosCompact, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
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

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.Compact != nil &&
		mco.Spec.AdvancedConfig.Compact.ServiceAccountAnnotations != nil {
		compactSpec.ServiceAccountAnnotations = mco.Spec.AdvancedConfig.Compact.ServiceAccountAnnotations
	}

	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.Compact != nil &&
		mco.Spec.AdvancedConfig.Compact.Containers != nil {
		compactSpec.Containers = mco.Spec.AdvancedConfig.Compact.Containers
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
		Resources: v1.VolumeResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceStorage: resource.MustParse(size),
			},
		},
	}
	return vct
}

func deleteStoreSts(cl client.Client, name string, oldNum int32, newNum int32) error {
	if oldNum > newNum {
		for i := newNum; i < oldNum; i++ {
			stsName := fmt.Sprintf("%s-thanos-store-shard-%d", name, i)
			found := &appsv1.StatefulSet{}
			err := cl.Get(
				context.TODO(),
				types.NamespacedName{Name: stsName, Namespace: mcoconfig.GetDefaultNamespace()},
				found,
			)
			if err != nil {
				if !k8serrors.IsNotFound(err) {
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

func addBackupLabel(c client.Client, name string, backupS *v1.Secret) error {
	if _, ok := mcoconfig.BackupResourceMap[name]; !ok {
		log.Info("Adding backup label", "Secret", name)
		mcoconfig.BackupResourceMap[name] = mcoconfig.ResourceTypeSecret
		var err error
		if backupS == nil {
			err = mcoutil.AddBackupLabelToSecret(c, name, mcoconfig.GetDefaultNamespace())
		} else {
			err = mcoutil.AddBackupLabelToSecretObj(c, backupS)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
