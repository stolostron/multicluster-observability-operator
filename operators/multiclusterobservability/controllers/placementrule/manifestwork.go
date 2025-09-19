// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"golang.org/x/exp/slices"
	rbacv1 "k8s.io/api/rbac/v1"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"

	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	cert_controller "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/certificates"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

const (
	workNameSuffix            = "-observability"
	workPostponeDeleteAnnoKey = "open-cluster-management/postpone-delete"
)

// intermediate resources for the manifest work.
var (
	hubInfoSecret             *corev1.Secret
	pullSecret                *corev1.Secret
	metricsAllowlistConfigMap *corev1.ConfigMap
	amAccessorTokenSecret     *corev1.Secret

	obsAddonCRDv1                 *apiextensionsv1.CustomResourceDefinition
	obsAddonCRDv1beta1            *apiextensionsv1beta1.CustomResourceDefinition
	endpointMetricsOperatorDeploy *appsv1.Deployment
	imageListConfigMap            *corev1.ConfigMap

	rawExtensionList []runtime.RawExtension
)

func deleteManifestWork(c client.Client, name string, namespace string) error {

	addon := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := c.Delete(context.TODO(), addon)
	if err != nil && !k8serrors.IsNotFound(err) {
		log.Error(err, "Failed to delete manifestworks", "name", name, "namespace", namespace)
		return err
	}
	return nil
}

func deleteManifestWorks(c client.Client, namespace string) error {

	err := c.DeleteAllOf(context.TODO(), &workv1.ManifestWork{},
		client.InNamespace(namespace), client.MatchingLabels{ownerLabelKey: ownerLabelValue})
	if err != nil {
		log.Error(err, "Failed to delete observability manifestworks", "namespace", namespace)
	}
	return err
}

func injectIntoWork(works []workv1.Manifest, obj runtime.Object) []workv1.Manifest {
	works = append(works,
		workv1.Manifest{
			RawExtension: runtime.RawExtension{
				Object: obj,
			},
		})
	return works
}

func newManifestwork(name string, namespace string) *workv1.ManifestWork {
	return &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
				// Add label expected by OCM to retrieve manifestWork belonging to the addon and update its status
				addonv1alpha1.AddonLabelKey: config.ManagedClusterAddonName,
			},
			Annotations: map[string]string{
				// Add the postpone delete annotation for manifestwork so that the observabilityaddon can be
				// cleaned up before the manifestwork is deleted by the managedcluster-import-controller when
				// the corresponding managedcluster is detached.
				// Note the annotation value is currently not taking effect, because managedcluster-import-controller
				// managedcluster-import-controller hard code the value to be 10m
				workPostponeDeleteAnnoKey: "",
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{},
			},
		},
	}
}

// removePostponeDeleteAnnotationForManifestwork removes the postpone delete annotation for manifestwork so that
// the workagent can delete the manifestwork normally
func removePostponeDeleteAnnotationForManifestwork(c client.Client, namespace string) error {
	name := namespace + workNameSuffix
	found := &workv1.ManifestWork{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil {
		log.Error(err, "failed to check manifestwork", "namespace", namespace, "name", name)
		return err
	}

	if found.GetAnnotations() != nil {
		delete(found.GetAnnotations(), workPostponeDeleteAnnoKey)
	}

	err = c.Update(context.TODO(), found)
	if err != nil {
		log.Error(err, "failed to update manifestwork", "namespace", namespace, "name", name)
		return err
	}

	return nil
}

func createManifestwork(ctx context.Context, c client.Client, work *workv1.ManifestWork) error {
	name := work.ObjectMeta.Name
	namespace := work.ObjectMeta.Namespace
	found := &workv1.ManifestWork{}
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		log.Info("Creating manifestwork", "namespace", namespace, "name", name)

		err = c.Create(ctx, work)
		if err != nil {
			logSizeErrorDetails(fmt.Sprint(err), work)
			return fmt.Errorf("failed to create manifestwork %s/%s: %w", namespace, name, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check manifestwork %s/%s: %w", namespace, name, err)
	}

	if found.GetDeletionTimestamp() != nil {
		log.Info("Existing manifestwork is terminating, skip and reconcile later")
		return errors.New("existing manifestwork is terminating, skip and reconcile later")
	}

	if !shouldUpdateManifestWork(work, found) {
		log.Info("manifestwork already existed/unchanged", "namespace", namespace)
		return nil
	}

	log.Info("Updating manifestwork", "namespace", namespace, "name", name)
	found.SetLabels(work.Labels)
	found.Spec.Workload.Manifests = work.Spec.Workload.Manifests
	err = c.Update(ctx, found)
	if err != nil {
		logSizeErrorDetails(fmt.Sprint(err), work)
		return fmt.Errorf("failed to update manifestwork %s/%s: %w", namespace, name, err)
	}
	return nil
}

func shouldUpdateManifestWork(desiredWork, foundWork *workv1.ManifestWork) bool {
	foundManifests := foundWork.Spec.Workload.Manifests
	desiredManifests := desiredWork.Spec.Workload.Manifests

	if !maps.Equal(foundWork.Labels, desiredWork.Labels) {
		return true
	}

	if len(desiredManifests) != len(foundManifests) {
		return true
	}

	for i, m := range foundManifests {
		if !util.CompareObject(m.RawExtension, desiredManifests[i].RawExtension) {
			return true
		}
	}

	return false
}

// generateGlobalManifestResources generates global resources, eg. manifestwork,
// endpoint-metrics-operator deploy and hubInfo Secret...
// this function is expensive and should not be called for each reconcile loop.
func generateGlobalManifestResources(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability) (
	[]workv1.Manifest, *workv1.Manifest, error) {

	works := []workv1.Manifest{}

	// inject the namespace
	works = injectIntoWork(works, generateNamespace())

	// inject the image pull secret
	if pullSecret == nil {
		var err error
		if pullSecret, err = generatePullSecret(c, config.GetImagePullSecret(mco.Spec)); err != nil {
			return nil, nil, err
		}
	}

	// inject the certificates
	managedClusterObsCert, err := generateObservabilityServerCACerts(ctx, c)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate observability server ca certs: %w", err)
	}
	works = injectIntoWork(works, managedClusterObsCert)

	// generate the metrics allowlist configmap
	if metricsAllowlistConfigMap == nil {
		var err error
		if metricsAllowlistConfigMap, err = generateMetricsListCM(c); err != nil {
			return nil, nil, fmt.Errorf("failed to generate metrics list configmap: %w", err)
		}
	}

	// inject the alertmanager accessor bearer token secret
	if amAccessorTokenSecret == nil {
		var err error
		if amAccessorTokenSecret, err = generateAmAccessorTokenSecret(c); err != nil {
			return nil, nil, err
		}
	}
	works = injectIntoWork(works, amAccessorTokenSecret)

	// reload resources if empty
	if len(rawExtensionList) == 0 || obsAddonCRDv1 == nil || obsAddonCRDv1beta1 == nil {
		var err error
		rawExtensionList, obsAddonCRDv1, obsAddonCRDv1beta1,
			endpointMetricsOperatorDeploy, imageListConfigMap, err = loadTemplates(mco)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load templates: %w", err)
		}
	}
	// inject resouces in templates
	crdv1Work := &workv1.Manifest{RawExtension: runtime.RawExtension{
		Object: obsAddonCRDv1,
	}}
	for _, raw := range rawExtensionList {
		works = append(works, workv1.Manifest{RawExtension: raw})
	}

	return works, crdv1Work, nil
}

// createManifestWorks creates a manifest work containing:
// - the spoke observability addon
// - the endpoint metrics operator
// - imageList configMap
// - pull secret
// - from the arg: the allowList, works, crdWork, hubInfo
func createManifestWorks(
	c client.Client,
	clusterNamespace string,
	cluster managedClusterInfo,
	mco *mcov1beta2.MultiClusterObservability,
	works []workv1.Manifest,
	allowlist *corev1.ConfigMap,
	crdWork *workv1.Manifest,
	dep *appsv1.Deployment,
	hubInfo *corev1.Secret,
	addonConfig *addonv1alpha1.AddOnDeploymentConfig,
	installProm bool,
) (*workv1.ManifestWork, error) {
	work := newManifestwork(clusterNamespace+workNameSuffix, clusterNamespace)

	manifests := work.Spec.Workload.Manifests
	// inject observabilityAddon
	obaddon, err := getObservabilityAddon(c, clusterNamespace, mco)
	if err != nil {
		return nil, err
	}
	if obaddon != nil {
		manifests = injectIntoWork(manifests, obaddon)
	}

	manifests = append(manifests, works...)
	manifests = injectIntoWork(manifests, allowlist)

	if !cluster.IsLocalCluster {
		manifests = append(manifests, *crdWork)
	}

	// replace the managedcluster image with the custom registry
	managedClusterImageRegistryMutex.RLock()
	_, hasCustomRegistry := managedClusterImageRegistry[cluster.Name]
	managedClusterImageRegistryMutex.RUnlock()
	imageRegistryClient := NewImageRegistryClient(c)

	// inject the endpoint operator deployment
	endpointMetricsOperatorDeployCopy := dep.DeepCopy()
	spec := endpointMetricsOperatorDeployCopy.Spec.Template.Spec
	if addonConfig.Spec.NodePlacement != nil {
		spec.NodeSelector = addonConfig.Spec.NodePlacement.NodeSelector
		spec.Tolerations = addonConfig.Spec.NodePlacement.Tolerations
	} else if cluster.IsLocalCluster {
		spec.NodeSelector = mco.Spec.NodeSelector
		spec.Tolerations = mco.Spec.Tolerations
	} else {
		// reset NodeSelector and Tolerations
		spec.NodeSelector = map[string]string{}
		spec.Tolerations = []corev1.Toleration{}
	}
	CustomCABundle := false
	for i, container := range spec.Containers {
		if container.Name == "endpoint-observability-operator" {
			for j, env := range container.Env {
				if env.Name == "HUB_NAMESPACE" {
					container.Env[j].Value = clusterNamespace
				}
				if env.Name == operatorconfig.InstallPrometheus {
					container.Env[j].Value = strconv.FormatBool(installProm)
				}
			}
			// If ProxyConfig is specified as part of addonConfig, set the proxy envs
			if !cluster.IsLocalCluster {
				for i := range spec.Containers {
					container := &spec.Containers[i]
					if addonConfig.Spec.ProxyConfig.HTTPProxy != "" {
						container.Env = append(container.Env, corev1.EnvVar{
							Name:  "HTTP_PROXY",
							Value: addonConfig.Spec.ProxyConfig.HTTPProxy,
						})
					}
					if addonConfig.Spec.ProxyConfig.HTTPSProxy != "" {
						container.Env = append(container.Env, corev1.EnvVar{
							Name:  "HTTPS_PROXY",
							Value: addonConfig.Spec.ProxyConfig.HTTPSProxy,
						})
						// CA is allowed only when HTTPS proxy is set
						if addonConfig.Spec.ProxyConfig.CABundle != nil {
							CustomCABundle = true
							container.Env = append(container.Env, corev1.EnvVar{
								Name:  "HTTPS_PROXY_CA_BUNDLE",
								Value: base64.StdEncoding.EncodeToString(addonConfig.Spec.ProxyConfig.CABundle),
							})
						}
					}
					if addonConfig.Spec.ProxyConfig.NoProxy != "" {
						container.Env = append(container.Env, corev1.EnvVar{
							Name:  "NO_PROXY",
							Value: addonConfig.Spec.ProxyConfig.NoProxy,
						})
					}
				}
			}

			if hasCustomRegistry {
				oldImage := container.Image
				newImage, err := imageRegistryClient.Cluster(cluster.Name).ImageOverride(oldImage)
				log.Info("Replace the endpoint operator image", "cluster", cluster.Name, "newImage", newImage)
				if err == nil {
					spec.Containers[i].Image = newImage
				}
			}
		}
	}
	if CustomCABundle {
		for i, manifest := range manifests {
			if manifest.RawExtension.Object.GetObjectKind().GroupVersionKind().Kind == "Secret" {
				secret := manifest.RawExtension.Object.DeepCopyObject().(*corev1.Secret)
				if secret.Name == managedClusterObsCertName {
					secret.Data["customCa.crt"] = addonConfig.Spec.ProxyConfig.CABundle
					manifests[i].RawExtension.Object = secret
					break
				}
			}
		}
	}

	log.Info(fmt.Sprintf("Cluster: %+v, Spec.NodeSelector (after): %+v", cluster.Name, spec.NodeSelector))
	log.Info(fmt.Sprintf("Cluster: %+v, Spec.Tolerations (after): %+v", cluster.Name, spec.Tolerations))

	if cluster.IsLocalCluster {
		spec.Volumes = []corev1.Volume{}
		spec.Containers[0].VolumeMounts = []corev1.VolumeMount{}
		for i, env := range spec.Containers[0].Env {
			if env.Name == "HUB_KUBECONFIG" {
				spec.Containers[0].Env[i].Value = ""
				break
			}
		}
		// Set HUB_ENDPOINT_OPERATOR when the endpoint operator is installed in hub cluster
		spec.Containers[0].Env = append(spec.Containers[0].Env, corev1.EnvVar{
			Name:  "HUB_ENDPOINT_OPERATOR",
			Value: "true",
		})

		dep.ObjectMeta.Name = config.HubEndpointOperatorName
	}
	endpointMetricsOperatorDeployCopy.Spec.Template.Spec = spec
	manifests = injectIntoWork(manifests, endpointMetricsOperatorDeployCopy)
	// replace the pull secret and addon components image
	if hasCustomRegistry {
		log.Info("Replace the default pull secret to custom pull secret", "cluster", cluster.Name)
		customPullSecret, err := imageRegistryClient.Cluster(cluster.Name).PullSecret()
		if err == nil && customPullSecret != nil {
			customPullSecret.ResourceVersion = ""
			customPullSecret.Name = config.GetImagePullSecret(mco.Spec)
			customPullSecret.Namespace = spokeNameSpace
			manifests = injectIntoWork(manifests, customPullSecret)
		}

		log.Info("Replace the image list configmap with custom image", "cluster", cluster.Name)
		newImageListCM := imageListConfigMap.DeepCopy()
		images := newImageListCM.Data
		for key, oldImage := range images {
			newImage, err := imageRegistryClient.Cluster(cluster.Name).ImageOverride(oldImage)
			if err == nil {
				newImageListCM.Data[key] = newImage
			}
		}
		manifests = injectIntoWork(manifests, newImageListCM)
	}

	if pullSecret != nil && !hasCustomRegistry {
		manifests = injectIntoWork(manifests, pullSecret)
	}

	if !hasCustomRegistry {
		manifests = injectIntoWork(manifests, imageListConfigMap)
	}

	// inject the hub info secret
	hubInfo.Data[operatorconfig.ClusterNameKey] = []byte(cluster.Name)
	manifests = injectIntoWork(manifests, hubInfo)

	work.Spec.Workload.Manifests = manifests

	return work, nil
}

func ensureResourcesForHubMetricsCollection(ctx context.Context, c client.Client, owner client.Object, manifests []workv1.Manifest) error {
	if operatorconfig.IsMCOTerminating {
		log.Info("MCO Operator is terminating, skip creating resources for hub metrics collection")
		return nil
	}

	// Make a deep copy of all the manifests since there are some global resources that can be updated due to this function
	objectToDeploy := make([]client.Object, 0, len(manifests))
	keepListKind := []string{"Deployment", "Secret", "ConfigMap", "ServiceAccount", "ClusterRole", "ClusterRoleBinding"}
	for _, manifest := range manifests {
		obj, ok := manifest.RawExtension.Object.DeepCopyObject().(client.Object)
		if !ok {
			log.Info("failed casting manaifest object as client.Object", "kind", manifest.Object.GetObjectKind())
			continue
		}

		kind := obj.GetObjectKind().GroupVersionKind().Kind
		if !slices.Contains(keepListKind, kind) {
			continue
		}

		// Ignore allow list configmaps as the hub ones are not reconciled by the placement controller
		if kind == "ConfigMap" && (obj.GetName() == operatorconfig.AllowlistConfigMapName || obj.GetName() == operatorconfig.AllowlistCustomConfigMapName) {
			continue
		}

		if err := controllerutil.SetControllerReference(owner, obj, c.Scheme()); err != nil {
			return fmt.Errorf("failed to set controller reference on object: %w", err)
		}

		//if kind is a Service account set the name as HubServiceAccount
		if kind == "ServiceAccount" {
			obj.SetName(config.HubEndpointSaName)
		}

		if kind == "ClusterRoleBinding" {
			if role, ok := obj.(*rbacv1.ClusterRoleBinding); ok {
				obj.SetName(config.HubEndpointRoleBindingName)
				if len(role.Subjects) > 0 {
					role.Subjects[0].Name = config.HubEndpointSaName
				}
			}
		}

		// set the service account name in the deployment to HubServiceAccount
		if kind == "Deployment" {
			if deploy, ok := obj.(*appsv1.Deployment); ok {
				deploy.Spec.Template.Spec.ServiceAccountName = config.HubEndpointSaName
			}
		}

		setHubNamespace(obj)
		objectToDeploy = append(objectToDeploy, obj)
	}

	for _, obj := range objectToDeploy {
		res, err := ctrl.CreateOrUpdate(ctx, c, obj, mutateHubResourceFn(obj.DeepCopyObject().(client.Object), obj))
		if err != nil {
			return fmt.Errorf("failed to create or update resource %s: %w", obj.GetName(), err)
		}
		if res != controllerutil.OperationResultNone {
			log.Info("resource created or updated", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName(), "action", res)
		}
	}

	err := cert_controller.CreateUpdateMtlsCertSecretForHubCollector(ctx, c)
	if err != nil {
		log.Error(err, "Failed to create client cert secret for hub metrics collection")
		return err
	}

	return nil
}

func setHubNamespace(obj client.Object) {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if kind == "Namespace" || kind == "ObservabilityAddon" || kind == "ClusterRole" || kind == "CustomResourceDefinition" {
		return
	}

	if role, ok := obj.(*rbacv1.ClusterRoleBinding); ok {
		if len(role.Subjects) > 0 {
			role.Subjects[0].Namespace = config.GetDefaultNamespace()
		}
		return
	}

	obj.SetNamespace(config.GetDefaultNamespace())
}

func mutateHubResourceFn(want, existing client.Object) controllerutil.MutateFn {
	return func() error {
		existing.SetOwnerReferences(want.GetOwnerReferences())
		switch existingTyped := existing.(type) {
		case *appsv1.Deployment:
			existingTyped.Spec = want.(*appsv1.Deployment).Spec
		case *corev1.Secret:
			existingTyped.Data = want.(*corev1.Secret).Data
		case *corev1.ConfigMap:
			existingTyped.Data = want.(*corev1.ConfigMap).Data
		case *rbacv1.ClusterRole:
			existingTyped.Rules = want.(*rbacv1.ClusterRole).Rules
		case *rbacv1.ClusterRoleBinding:
			existingTyped.Subjects = want.(*rbacv1.ClusterRoleBinding).Subjects
		case *corev1.ServiceAccount:
			mutateServiceAccount(want.(*corev1.ServiceAccount), existingTyped)
		}
		return nil
	}
}

func mutateServiceAccount(want, existing *corev1.ServiceAccount) {
	// https://issues.redhat.com/browse/ACM-10967
	// Some of these ServiceAccounts will be read from static files so they will never contain
	// the generated Secrets as part of their corev1.ServiceAccount.ImagePullSecrets field.
	// This checks by way of slice length if this particular ServiceAccount can be one of those.
	if len(want.ImagePullSecrets) < len(existing.ImagePullSecrets) {
		for _, imagePullSecret := range want.ImagePullSecrets {
			if !slices.Contains(existing.ImagePullSecrets, imagePullSecret) {
				existing.ImagePullSecrets = want.ImagePullSecrets
				break
			}
		}
	} else {
		sortObjRef := func(a, b corev1.ObjectReference) bool {
			return a.Name < b.Name
		}

		sortLocalObjRef := func(a, b corev1.LocalObjectReference) bool {
			return a.Name < b.Name
		}

		cmpOptions := []gocmp.Option{gocmpopts.EquateEmpty(), gocmpopts.SortSlices(sortObjRef), gocmpopts.SortSlices(sortLocalObjRef)}
		if !gocmp.Equal(want.ImagePullSecrets, existing.ImagePullSecrets, cmpOptions...) {
			existing.ImagePullSecrets = want.ImagePullSecrets
		}
	}
}

// Delete resources created for hub metrics collection
func DeleteHubMetricsCollectionDeployments(ctx context.Context, c client.Client) error {
	if err := DeleteHubMetricsCollectorResourcesNotNeededForMCOA(ctx, c); err != nil {
		return fmt.Errorf("failed to delete MCOA resources: %w", err)
	}

	toDelete := []client.Object{
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{ // hub observability api CA cert
			Name:      managedClusterObsCertName,
			Namespace: config.GetDefaultNamespace(),
		}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{ // token for sending alerts to hub alertmanager from in-cluster prometheus
			Name:      config.AlertmanagerAccessorSecretName,
			Namespace: config.GetDefaultNamespace(),
		}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{ // cert and key for mTLS connection with hub observability api
			Name:      operatorconfig.HubMetricsCollectorMtlsCert,
			Namespace: config.GetDefaultNamespace(),
		}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{ // images list configmap
			Name:      operatorconfig.ImageConfigMap,
			Namespace: config.GetDefaultNamespace(),
		}},
	}

	for _, obj := range toDelete {
		if err := deleteObject(ctx, c, obj); err != nil {
			return err
		}
	}

	return nil
}

// DeleteHubMetricsCollectorResourcesNotNeededForMCOA deletes hub resources for the metrics collector but keeps the ones
// common to MCOA and the metrics collector.
func DeleteHubMetricsCollectorResourcesNotNeededForMCOA(ctx context.Context, c client.Client) error {
	toDelete := []client.Object{
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{ // hub endpoint operator
			Name:      config.HubEndpointOperatorName,
			Namespace: config.GetDefaultNamespace(),
		}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{ // uwl metrics collector
			Name:      config.HubUwlMetricsCollectorName,
			Namespace: config.GetDefaultNamespace(),
		}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{ // platform metrics collector
			Name:      config.HubMetricsCollectorName,
			Namespace: config.GetDefaultNamespace(),
		}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{ // CA cert for service-serving certificates
			Name:      operatorconfig.CaConfigmapName,
			Namespace: config.GetDefaultNamespace(),
		}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{ // hub info secret
			Name:      operatorconfig.HubInfoSecretName,
			Namespace: config.GetDefaultNamespace(),
		}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{ // metrics-collector-view role
			Name: clusterRoleBindingName,
		}},
	}

	for _, obj := range toDelete {
		if err := deleteObject(ctx, c, obj); err != nil {
			return err
		}
	}

	err := RevertHubClusterMonitoringConfig(ctx, c)
	if err != nil {
		return fmt.Errorf("failed to revert cluster monitoring config: %w", err)
	}

	return nil
}

func deleteObject[T client.Object](ctx context.Context, c client.Client, obj T) error {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return fmt.Errorf("could not determine GVK for object: %w", err)
	}

	if err := c.Delete(ctx, obj); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("failed to delete object %s %s/%s: %w", gvk.Kind, obj.GetNamespace(), obj.GetName(), err)
	}

	log.Info("Deleted object", "gvk", gvk.String(), "namespace", obj.GetNamespace(), "name", obj.GetName())
	return nil
}

// generateAmAccessorTokenSecret generates the secret that contains the access_token
// for the Alertmanager in the Hub cluster
func generateAmAccessorTokenSecret(cl client.Client) (*corev1.Secret, error) {
	amAccessorSA := &corev1.ServiceAccount{}
	err := cl.Get(context.TODO(), types.NamespacedName{Name: config.AlertmanagerAccessorSAName,
		Namespace: config.GetDefaultNamespace()}, amAccessorSA)
	if err != nil {
		log.Error(err, "Failed to get Alertmanager accessor serviceaccount", "name", config.AlertmanagerAccessorSAName)
		return nil, err
	}

	tokenSrtName := ""
	for _, secretRef := range amAccessorSA.Secrets {
		if strings.HasPrefix(secretRef.Name, config.AlertmanagerAccessorSAName+"-token") {
			tokenSrtName = secretRef.Name
			break
		}
	}

	if tokenSrtName == "" {
		// Starting with kube 1.24 (ocp 4.11), the k8s won't generate secrets any longer
		// automatically for ServiceAccounts, for OCP, when a service account is created,
		// the OCP will create two secrets, one stores dockercfg with name format (<sa name>-dockercfg-<random>)
		// and the other stores the service account token  with name format (<sa name>-token-<random>),
		// but the service account secrets won't list in the service account any longer.
		secretList := &corev1.SecretList{}
		err = cl.List(context.TODO(), secretList, &client.ListOptions{Namespace: config.GetDefaultNamespace()})
		if err != nil {
			return nil, err
		}

		for _, secret := range secretList.Items {
			if secret.Type == corev1.SecretTypeServiceAccountToken &&
				strings.HasPrefix(secret.Name, config.AlertmanagerAccessorSAName+"-token") {
				tokenSrtName = secret.Name
				break
			}
		}
		// since we do not want to rely on the behavior above from OCP
		// as the docs hint that it will be removed in the future
		// if we do not find the token secret, we will create the Secret ourselves
		// which should be picked up in the next reconcile loop
		if tokenSrtName == "" {
			secretName := config.AlertmanagerAccessorSAName + "-token"
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: config.GetDefaultNamespace(),
					Annotations: map[string]string{
						"kubernetes.io/service-account.name": amAccessorSA.Name,
					},
				},
				Type: "kubernetes.io/service-account-token",
			}
			err := cl.Create(context.TODO(), secret, &client.CreateOptions{})
			if err != nil && !k8serrors.IsAlreadyExists(err) {
				log.Error(err, "Failed to create token secret for Alertmanager accessor serviceaccount",
					"name", config.AlertmanagerAccessorSAName)
				return nil, err
			}
			log.Info(
				"Created secret for Alertmanager accessor serviceaccount",
				"name",
				secretName,
				"namespace",
				config.GetDefaultNamespace(),
			)
			tokenSrtName = secretName
		}
	}

	tokenSrt := &corev1.Secret{}
	err = cl.Get(context.TODO(), types.NamespacedName{Name: tokenSrtName,
		Namespace: config.GetDefaultNamespace()}, tokenSrt)
	if err != nil {
		log.Error(err, "Failed to get token secret for Alertmanager accessor serviceaccount", "name", tokenSrtName)
		return nil, err
	}

	data, ok := tokenSrt.Data["token"]
	if !ok || len(data) == 0 {
		err = fmt.Errorf("service account token not populated or empty: %s", config.AlertmanagerAccessorSAName)
		log.Error(
			err,
			"no token present in Secret for Alertmanager accessor serviceaccount",
			"service account name",
			config.AlertmanagerAccessorSAName,
			"secret name",
			tokenSrtName,
		)
		return nil, err
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerAccessorSecretName,
			Namespace: spokeNameSpace,
		},
		Data: map[string][]byte{
			"token": tokenSrt.Data["token"],
		},
	}, nil
}

// generatePullSecret generates the image pull secret for mco
func generatePullSecret(c client.Client, name string) (*corev1.Secret, error) {
	imagePullSecret := &corev1.Secret{}
	err := c.Get(context.TODO(),
		types.NamespacedName{
			Name:      name,
			Namespace: config.GetDefaultNamespace(),
		}, imagePullSecret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		} else {
			log.Error(err, "Failed to get the pull secret", "name", name)
			return nil, err
		}
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      imagePullSecret.Name,
			Namespace: spokeNameSpace,
		},
		Data: map[string][]byte{
			".dockerconfigjson": imagePullSecret.Data[".dockerconfigjson"],
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}, nil
}

// generateObservabilityServerCACerts extracts the CA cert from the secret holding the observability TLS certs
// and returns a secret to be deployed on spokes containing it.
func generateObservabilityServerCACerts(ctx context.Context, client client.Client) (*corev1.Secret, error) {
	ca := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{Name: config.ServerCACerts, Namespace: config.GetDefaultNamespace()}, ca)
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedClusterObsCertName,
			Namespace: spokeNameSpace,
		},
		Data: map[string][]byte{
			"ca.crt": ca.Data["tls.crt"],
		},
	}, nil
}

// generateMetricsListCM generates the configmap that contains the metrics allowlist
func generateMetricsListCM(client client.Client) (*corev1.ConfigMap, error) {
	metricsAllowlistCM := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.AllowlistConfigMapName,
			Namespace: spokeNameSpace,
		},
		Data: map[string]string{},
	}

	allowlist, uwlAllowlist, err := util.GetAllowList(client,
		operatorconfig.AllowlistConfigMapName, config.GetDefaultNamespace())
	if err != nil {
		log.Error(err, "Failed to get metrics allowlist configmap "+operatorconfig.AllowlistConfigMapName)
		return nil, err
	}

	customAllowlist, customUwlAllowlist, err := util.GetAllowList(client,
		config.AllowlistCustomConfigMapName, config.GetDefaultNamespace())
	if err == nil {
		allowlist, uwlAllowlist = util.MergeAllowlist(allowlist,
			customAllowlist, uwlAllowlist, customUwlAllowlist)
	} else {
		log.Info("There is no custom metrics allowlist configmap in the cluster")
	}

	data, err := yaml.Marshal(allowlist)
	if err != nil {
		log.Error(err, "Failed to marshal allowlist data")
		return nil, err
	}
	uwlData, err := yaml.Marshal(uwlAllowlist)
	if err != nil {
		log.Error(err, "Failed to marshal allowlist uwlAllowlist")
		return nil, err
	}
	metricsAllowlistCM.Data[operatorconfig.MetricsConfigMapKey] = string(data)
	metricsAllowlistCM.Data[operatorconfig.UwlMetricsConfigMapKey] = string(uwlData)

	return metricsAllowlistCM, nil
}

// getObservabilityAddon gets the ObservabilityAddon in the spoke namespace in the hub cluster.
// This is then synced to the actual spoke, by injecting it into the manifestwork.
// We assume that an existing addon will always be found here as we create it initially.
// If the addon is found with the mco source annotation, it will update the existing addon with the new values from MCO
// If the addon is found with the override source annotation, it will not update the existing addon but it will use the existing values.
// If the addon is found without any source annotation, it will add the mco source annotation and use the MCO values (upgrade case from ACM 2.12.2).
func getObservabilityAddon(c client.Client, namespace string,
	mco *mcov1beta2.MultiClusterObservability) (*mcov1beta1.ObservabilityAddon, error) {
	if namespace == config.GetDefaultNamespace() {
		return nil, nil
	}
	found := &mcov1beta1.ObservabilityAddon{}
	namespacedName := types.NamespacedName{
		Name:      obsAddonName,
		Namespace: namespace,
	}
	err := c.Get(context.TODO(), namespacedName, found)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err, "Failed to check observabilityAddon")
			return nil, err
		}
	}
	if found.ObjectMeta.DeletionTimestamp != nil {
		return nil, nil
	}

	addon := &mcov1beta1.ObservabilityAddon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "observability.open-cluster-management.io/v1beta1",
			Kind:       "ObservabilityAddon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        obsAddonName,
			Namespace:   spokeNameSpace,
			Annotations: make(map[string]string),
		},
	}

	// Handle cases where the addon doesn't have the annotation
	if found.Annotations == nil {
		found.Annotations = make(map[string]string)
	}

	if _, ok := found.Annotations[addonSourceAnnotation]; !ok {
		found.Annotations[addonSourceAnnotation] = addonSourceMCO
	}

	addon.Annotations = found.Annotations

	if found.Annotations[addonSourceAnnotation] == addonSourceMCO {
		setObservabilityAddonSpec(addon, mco.Spec.ObservabilityAddonSpec, config.GetOBAResources(mco.Spec.ObservabilityAddonSpec, mco.Spec.InstanceSize))
	}

	if found.Annotations[addonSourceAnnotation] == addonSourceOverride {
		setObservabilityAddonSpec(addon, &found.Spec, found.Spec.Resources)
	}

	return addon, nil
}

func removeObservabilityAddonInManifestWork(ctx context.Context, client client.Client, namespace string) error {
	name := namespace + workNameSuffix
	found := &workv1.ManifestWork{}
	err := client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("failed to get manifestwork %s/%s: %w", namespace, name, err)
	}

	updateManifests := slices.DeleteFunc(slices.Clone(found.Spec.Workload.Manifests), func(e workv1.Manifest) bool {
		obj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, e.Raw)
		if err != nil {
			log.Error(err, "Failed to decode manifest item")
			return false
		}
		return obj.GetObjectKind().GroupVersionKind().Kind == "ObservabilityAddon"
	})

	if len(updateManifests) != len(found.Spec.Workload.Manifests) {
		found.Spec.Workload.Manifests = updateManifests
		log.Info("Removing ObservabilityAddon from ManifestWork", "name", name, "namespace", namespace, "removed_objects", len(found.Spec.Workload.Manifests)-len(updateManifests), "objects_count", len(updateManifests))
		if err := client.Update(ctx, found); err != nil {
			return fmt.Errorf("failed to update manifestwork %s/%s: %w", namespace, name, err)
		}
	}
	return nil
}

func logSizeErrorDetails(str string, work *workv1.ManifestWork) {
	if strings.Contains(str, "the size of manifests") {
		var keyVal []any
		for _, manifest := range work.Spec.Workload.Manifests {
			raw, _ := json.Marshal(manifest.RawExtension.Object)
			keyVal = append(keyVal, "kind", manifest.RawExtension.Object.GetObjectKind().
				GroupVersionKind().Kind, "size", len(raw))
		}
		log.Info("size of manifest", keyVal...)
	}
}
