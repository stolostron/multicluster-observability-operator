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
	"slices"
	"strconv"
	"strings"
	"time"

	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"
	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	cert_controller "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/certificates"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	workNameSuffix            = "-observability"
	workPostponeDeleteAnnoKey = "open-cluster-management/postpone-delete"
	amTokenExpiration         = "token-expiration"
	amTokenCreated            = "token-created"
)

// managedManifestWorkAnnotations explicitly lists the annotations we own on ManifestWork.
// We only update/remove annotations in this list, preserving all others to avoid
// conflicts with addon-framework and other OCM controllers.
//
// When adding a new annotation to ManifestWork in createManifestWorks(), add it to this list.
var managedManifestWorkAnnotations = []string{
	workPostponeDeleteAnnoKey,                  // "open-cluster-management/postpone-delete"
	workv1.ManifestConfigSpecHashAnnotationKey, // "open-cluster-management.io/config-spec-hash"
}

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

// getConfigSpecHashAnnotation retrieves the ManagedClusterAddOn and converts its configReferences
// to the annotation format expected by the addon-framework for ManifestWork.
// Returns the annotation key-value map, or nil if no configs are present.
func getConfigSpecHashAnnotation(ctx context.Context, c client.Client, namespace string) (map[string]string, error) {
	// Get the ManagedClusterAddOn for this cluster
	mca := &addonv1alpha1.ManagedClusterAddOn{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      config.ManagedClusterAddonName,
		Namespace: namespace,
	}, mca)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// MCA doesn't exist yet, no config annotation needed
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ManagedClusterAddOn: %w", err)
	}

	// If there are no configReferences, no annotation is needed
	if len(mca.Status.ConfigReferences) == 0 {
		return nil, nil
	}

	// Convert configReferences to the annotation format
	// Format: map[<resource>.<group>/<namespace>/<name>] = specHash
	specHashMap := make(map[string]string)
	for _, configRef := range mca.Status.ConfigReferences {
		if configRef.DesiredConfig == nil || configRef.DesiredConfig.SpecHash == "" {
			continue
		}

		// Build the config key: <resource>.<group>/<namespace>/<name>
		configKey := configRef.Resource
		if len(configRef.Group) > 0 {
			configKey += fmt.Sprintf(".%s", configRef.Group)
		}
		configKey += fmt.Sprintf("/%s/%s", configRef.DesiredConfig.Namespace, configRef.DesiredConfig.Name)

		specHashMap[configKey] = configRef.DesiredConfig.SpecHash
	}

	// If no valid configs with specHash, return nil
	if len(specHashMap) == 0 {
		return nil, nil
	}

	// Marshal the map to JSON
	jsonBytes, err := json.Marshal(specHashMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config spec hash map: %w", err)
	}

	// Return the annotation with the expected key
	return map[string]string{
		workv1.ManifestConfigSpecHashAnnotationKey: string(jsonBytes),
	}, nil
}

// removePostponeDeleteAnnotationForManifestwork removes the postpone delete annotation for manifestwork so that
// the workagent can delete the manifestwork normally
func removePostponeDeleteAnnotationForManifestwork(c client.Client, namespace string) error {
	name := namespace + workNameSuffix
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
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
			return err
		}

		return nil
	})
}

func createManifestwork(ctx context.Context, c client.Client, work *workv1.ManifestWork) error {
	if work.Namespace == config.GetDefaultNamespace() {
		return nil
	}
	name := work.Name
	namespace := work.Namespace
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
	// Only update annotations we manage, preserving annotations from other controllers
	updateManagedAnnotations(found, work)
	found.Spec.Workload.Manifests = work.Spec.Workload.Manifests
	err = c.Update(ctx, found)
	if err != nil {
		logSizeErrorDetails(fmt.Sprint(err), work)
		return fmt.Errorf("failed to update manifestwork %s/%s: %w", namespace, name, err)
	}
	return nil
}

// updateManagedAnnotations updates only the annotations we manage on the target ManifestWork,
// preserving any annotations set by other controllers. This prevents reconciliation conflicts.
func updateManagedAnnotations(target, source *workv1.ManifestWork) {
	if target.Annotations == nil {
		target.Annotations = make(map[string]string)
	}

	for _, key := range managedManifestWorkAnnotations {
		if val, exists := source.Annotations[key]; exists {
			// Set or update our annotation
			target.Annotations[key] = val
		} else {
			// Remove our annotation if we no longer set it
			delete(target.Annotations, key)
		}
	}
}

func shouldUpdateManifestWork(desiredWork, foundWork *workv1.ManifestWork) bool {
	foundManifests := foundWork.Spec.Workload.Manifests
	desiredManifests := desiredWork.Spec.Workload.Manifests

	if !maps.Equal(foundWork.Labels, desiredWork.Labels) {
		return true
	}

	// Only check annotations we manage to avoid triggering updates for annotations
	// set by other controllers
	for _, key := range managedManifestWorkAnnotations {
		desired, desiredExists := desiredWork.Annotations[key]
		found, foundExists := foundWork.Annotations[key]

		if desiredExists != foundExists || (desiredExists && desired != found) {
			return true
		}
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
func generateGlobalManifestResources(ctx context.Context, c client.Client, mco *mcov1beta2.MultiClusterObservability, kubeClient kubernetes.Interface) (
	[]workv1.Manifest, *workv1.Manifest, error,
) {
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
	amAccessorTokenSecret, err = generateAmAccessorTokenSecret(c, kubeClient)
	if err != nil {
		return nil, nil, err
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
	ctx context.Context,
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
	customCABundle := customizeEndpointOperator(
		endpointMetricsOperatorDeployCopy,
		mco,
		addonConfig,
		cluster,
		clusterNamespace,
		installProm,
		hasCustomRegistry,
		imageRegistryClient,
	)
	if err != nil {
		return nil, err
	}

	if customCABundle {
		for i, manifest := range manifests {
			if manifest.RawExtension.Object.GetObjectKind().GroupVersionKind().Kind == "Secret" {
				secret := manifest.Object.DeepCopyObject().(*corev1.Secret)
				if secret.Name == managedClusterObsCertName {
					secret.Data["customCa.crt"] = addonConfig.Spec.ProxyConfig.CABundle
					manifests[i].Object = secret
					break
				}
			}
		}
	}

	log.Info(fmt.Sprintf("Cluster: %+v, Spec.NodeSelector (after): %+v", cluster.Name, endpointMetricsOperatorDeployCopy.Spec.Template.Spec.NodeSelector))
	log.Info(fmt.Sprintf("Cluster: %+v, Spec.Tolerations (after): %+v", cluster.Name, endpointMetricsOperatorDeployCopy.Spec.Template.Spec.Tolerations))

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

	// Set the config spec hash annotation if the ManagedClusterAddOn has configReferences
	configAnnotation, err := getConfigSpecHashAnnotation(ctx, c, clusterNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get config spec hash annotation: %w", err)
	}
	maps.Copy(work.Annotations, configAnnotation)

	return work, nil
}

func customizeEndpointOperator(
	dep *appsv1.Deployment,
	mco *mcov1beta2.MultiClusterObservability,
	addonConfig *addonv1alpha1.AddOnDeploymentConfig,
	cluster managedClusterInfo,
	clusterNamespace string,
	installProm bool,
	hasCustomRegistry bool,
	imageRegistryClient Client,
) bool {
	spec := &dep.Spec.Template.Spec
	switch {
	case addonConfig.Spec.NodePlacement != nil:
		spec.NodeSelector = addonConfig.Spec.NodePlacement.NodeSelector
		spec.Tolerations = addonConfig.Spec.NodePlacement.Tolerations
	case cluster.IsLocalCluster:
		spec.NodeSelector = mco.Spec.NodeSelector
		spec.Tolerations = mco.Spec.Tolerations
	default:
		// reset NodeSelector and Tolerations
		spec.NodeSelector = map[string]string{}
		spec.Tolerations = []corev1.Toleration{}
	}

	customCABundle := false
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
				customCABundle = injectProxyConfig(spec, addonConfig)
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

		dep.Name = config.HubEndpointOperatorName
	}
	return customCABundle
}

func injectProxyConfig(spec *corev1.PodSpec, addonConfig *addonv1alpha1.AddOnDeploymentConfig) bool {
	customCABundle := false
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
				customCABundle = true
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
	return customCABundle
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
		obj, ok := manifest.Object.DeepCopyObject().(client.Object)
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

		// if kind is a Service account set the name as HubServiceAccount
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
			existingTyped.Annotations = want.(*corev1.Secret).Annotations
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
	err := DeleteHubMetricsCollectorResourcesNotNeededForMCOA(ctx, c)
	if err != nil {
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
	// First delete the endpoint-operator so that it doesn't override changes on the CMO config
	// Other resources are also deleted
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
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{ // metrics-collector-view role
			Name: clusterRoleBindingName,
		}},
	}

	for _, obj := range toDelete {
		if err := deleteObject(ctx, c, obj); err != nil {
			return err
		}
	}

	// This revert function depends on the hubInfoSecret, it must be executed before deleting it.
	err := RevertHubClusterMonitoringConfig(ctx, c)
	if err != nil {
		return fmt.Errorf("failed to revert hub cluster monitoring config: %w", err)
	}

	// Delete the hubInfoSecret after the CMO config revert
	toDelete = []client.Object{
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{ // hub info secret
			Name:      operatorconfig.HubInfoSecretName,
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
func generateAmAccessorTokenSecret(cl client.Client, kubeClient kubernetes.Interface) (*corev1.Secret, error) {
	if kubeClient == nil {
		return nil, fmt.Errorf("kubeClient is required but was nil")
	}

	// fetch any existing secrets from the hub-namespace if we didn't already
	// have a secret saved in the global variable. We use a global variable
	// because we might reconcile a spoke before the hub in which the spoke
	// has a token which is newer than the one in the hub namespace. This is
	// not strictly needed but we might as well create as few tokens as possible
	if amAccessorTokenSecret == nil {
		amAccessorTokenSecret = &corev1.Secret{}
		err := cl.Get(context.TODO(),
			types.NamespacedName{
				Name:      config.AlertmanagerAccessorSecretName,
				Namespace: config.GetDefaultNamespace(),
			}, amAccessorTokenSecret)
		if err != nil {
			// If it's not found, we'll create a new one later
			// we null the var here as it was set to an empty
			// secret above, which is required for the Get function
			if k8serrors.IsNotFound(err) {
				amAccessorTokenSecret = nil
			} else {
				return nil, fmt.Errorf("unable to lookup existing alertmanager accessor secret. %w", err)
			}
		}
	}

	// If we managed to find an existing secret, check the expiration
	if amAccessorTokenSecret != nil {
		expirationBytes, hasExpiration := amAccessorTokenSecret.Annotations[amTokenExpiration]
		createdBytes, hasCreated := amAccessorTokenSecret.Annotations[amTokenCreated]

		if hasExpiration && hasCreated {
			// Check if the token is near expiration
			expirationStr := expirationBytes
			expiration, err := time.Parse(time.RFC3339, expirationStr)
			if err != nil {
				log.Error(err, "Failed to parse alertmanager accessor token expiration date", "expiration", expiration)
				return nil, err
			}
			// find out the expected duration of the token
			createdStr := createdBytes
			created, err := time.Parse(time.RFC3339, createdStr)
			if err != nil {
				log.Error(err, "Failed to parse alertmanager accessor token creation date", "created", created)
				return nil, err
			}

			expectedDuration := expiration.Sub(created)
			if expectedDuration <= 0 {
				log.Error(nil, "Invalid duration for alertmanager accessor token", "duration", expectedDuration)
				return nil, nil
			}
			percentOfExp := float64(time.Until(expiration)) / float64(expectedDuration)
			// Current amAccessorTokenSecret is not near expiration, returning it
			if percentOfExp >= 0.2 {
				// We set the spoke namespace here to ensure it is created on the right
				// namespace on spokes, since we fetched it from the hub namespace which
				// is different. Further we set the ResourceVersion to be empty to ensure
				// if this is the first time we create the object on a spoke, it goes
				// without errors. When we are on the hub, the namespace for the secret
				// will later be overwritten in ensureResourcesForHubMetricsCollection
				amAccessorTokenSecret.SetNamespace(spokeNameSpace)
				amAccessorTokenSecret.SetResourceVersion("")
				return amAccessorTokenSecret, nil
			}
		}
	}

	// This creates a JWT token for the alertmanager service account.
	// This is used to verify incoming alertmanager connetions from prometheus.
	tokenRequest, err := kubeClient.CoreV1().ServiceAccounts(config.GetDefaultNamespace()).CreateToken(context.TODO(), config.AlertmanagerAccessorSAName, &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			ExpirationSeconds: ptr.To(int64(8640 * 3600)), // expires in 364 days
		},
	}, metav1.CreateOptions{})
	if err != nil {
		log.Error(err, "Failed to create token for Alertmanager accessor serviceaccount",
			"name", config.AlertmanagerAccessorSAName,
			"namespace", config.GetDefaultNamespace())
		return nil, err
	}

	// if it exists, delete the previous unbound token secret for the Alertmanager accessor service account
	err = deleteAlertmanagerAccessorTokenSecret(context.TODO(), cl)
	if err != nil {
		log.Error(err, "Failed to delete alertmanager accessor token secret")
	}

	now := time.Now()
	// Now create the secret for the spoke, with the JWT token
	// from the service account token request above.
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerAccessorSecretName,
			Namespace: spokeNameSpace,
			Annotations: map[string]string{
				amTokenExpiration: tokenRequest.Status.ExpirationTimestamp.Format(time.RFC3339),
				amTokenCreated:    now.Format(time.RFC3339),
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token": []byte(tokenRequest.Status.Token),
		},
	}, nil
}

// Note: This can be removed for 2.17
// deleteAlertmanagerAccessorTokenSecret deletes the previous unbound token secret
func deleteAlertmanagerAccessorTokenSecret(ctx context.Context, cl client.Client) error {
	secretToDelete := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AlertmanagerAccessorSAName + "-token",
			Namespace: config.GetDefaultNamespace(),
		},
	}

	err := cl.Delete(ctx, secretToDelete, &client.DeleteOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Unbound token secret not found
			return nil
		}
		return fmt.Errorf("failed to delete secret %s/%s: %w", secretToDelete.Namespace, secretToDelete.Name, err)
	}
	log.Info("Removed depricated alertmanager accessor token secret", "name", config.AlertmanagerAccessorSAName+"-token", "namespace", config.GetDefaultNamespace())

	return nil
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
		}
		log.Error(err, "Failed to get the pull secret", "name", name)
		return nil, err
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
	mco *mcov1beta2.MultiClusterObservability,
) (*mcov1beta1.ObservabilityAddon, error) {
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
	if found.DeletionTimestamp != nil {
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
		log.Info(
			"Removing ObservabilityAddon from ManifestWork",
			"name",
			name,
			"namespace",
			namespace,
			"removed_objects",
			len(found.Spec.Workload.Manifests)-len(updateManifests),
			"objects_count",
			len(updateManifests),
		)
		if err := client.Update(ctx, found); err != nil {
			return fmt.Errorf("failed to update manifestwork %s/%s: %w", namespace, name, err)
		}
	}
	return nil
}

func logSizeErrorDetails(str string, work *workv1.ManifestWork) {
	if strings.Contains(str, "the size of manifests") {
		keyVal := make([]any, 0, len(work.Spec.Workload.Manifests)*4)
		for _, manifest := range work.Spec.Workload.Manifests {
			raw, _ := json.Marshal(manifest.Object)
			keyVal = append(keyVal, "kind", manifest.RawExtension.Object.GetObjectKind().
				GroupVersionKind().Kind, "size", len(raw))
		}
		log.Info("size of manifest", keyVal...)
	}
}
