// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	workNameSuffix            = "-observability"
	localClusterName          = "local-cluster"
	workPostponeDeleteAnnoKey = "open-cluster-management/postpone-delete"
)

// intermidiate resources for the manifest work
var (
	hubInfoSecret                   *corev1.Secret
	pullSecret                      *corev1.Secret
	managedClusterObsCert           *corev1.Secret
	metricsAllowlistConfigMap       *corev1.ConfigMap
	ocp311metricsAllowlistConfigMap *corev1.ConfigMap
	amAccessorTokenSecret           *corev1.Secret

	obsAddonCRDv1                 *apiextensionsv1.CustomResourceDefinition
	obsAddonCRDv1beta1            *apiextensionsv1beta1.CustomResourceDefinition
	endpointMetricsOperatorDeploy *appsv1.Deployment
	imageListConfigMap            *corev1.ConfigMap

	rawExtensionList []runtime.RawExtension
	//promRawExtensionList []runtime.RawExtension
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

func createManifestwork(c client.Client, work *workv1.ManifestWork) error {
	name := work.ObjectMeta.Name
	namespace := work.ObjectMeta.Namespace
	log.Info(fmt.Sprintf("createManifestWork (name): %s, (namespace): %s", name, namespace))
	found := &workv1.ManifestWork{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		log.Info("Creating manifestwork", "namespace", namespace, "name", name)

		err = c.Create(context.TODO(), work)
		if err != nil {
			log.Error(err, "Failed to create manifestwork", "namespace", namespace, "name", name)
			logSizeErrorDetails(fmt.Sprint(err), work)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check manifestwork", namespace, "name", name)
		return err
	}

	if found.GetDeletionTimestamp() != nil {
		log.Info("Existing manifestwork is terminating, skip and reconcile later")
		return errors.New("existing manifestwork is terminating, skip and reconcile later")
	}

	manifests := work.Spec.Workload.Manifests
	updated := false
	if len(found.Spec.Workload.Manifests) == len(manifests) {
		for i, m := range found.Spec.Workload.Manifests {
			if !util.CompareObject(m.RawExtension, manifests[i].RawExtension) {
				updated = true
				break
			}
		}
	} else {
		updated = true
	}

	if updated {
		log.Info("Updating manifestwork", "namespace", namespace, "name", name)
		for _, m := range found.Spec.Workload.Manifests {
			var unstructuredObj unstructured.Unstructured
			var err error
			if json.Valid(m.Raw) {
				err = unstructuredObj.UnmarshalJSON(m.Raw)
				if err != nil {
					log.Info(fmt.Sprintf("could not unmarshal manifestwork as Json(found), name: %s, namespace: %s", name, namespace))
				}
			} else {
				err = yaml.Unmarshal(m.Raw, &unstructuredObj)
				if err != nil {
					log.Info(fmt.Sprintf("could not unmarshal manifestwork as Yaml(found), name: %s, namespace: %s", name, namespace))
				}
			}

			if err != nil {
				log.Info(fmt.Sprintf("m.Raw: %+v", m.Raw))
			} else {
				if unstructuredObj.GetKind() == "Deployment" {
					deploymentName := unstructuredObj.GetName()
					nodeSelector, _, _ := unstructured.NestedMap(unstructuredObj.Object, "spec", "template", "spec", "nodeSelector")
					tolerations, _, _ := unstructured.NestedSlice(unstructuredObj.Object, "spec", "template", "spec", "tolerations")
					log.Info(fmt.Sprintf("Manifestwork (found): %s, Deployment Name: %s", name, deploymentName))
					log.Info(fmt.Sprintf("Manifestwork (found): %+v, NodeSelector: %+v", name, nodeSelector))
					log.Info(fmt.Sprintf("Manifestwork (found): %+v, Tolerations: %+v", name, tolerations))
				}
			}
		}
		for _, m := range manifests {
			var unstructuredObj unstructured.Unstructured
			var err error
			if json.Valid(m.Raw) {
				err = unstructuredObj.UnmarshalJSON(m.Raw)
				if err != nil {
					log.Info(fmt.Sprintf("could not unmarshal manifestwork as Json(new), name: %s, namespace: %s", name, namespace))
				}
			} else {
				err = yaml.Unmarshal(m.Raw, &unstructuredObj)
				if err != nil {
					log.Info(fmt.Sprintf("could not unmarshal manifestwork as Yaml(new), name: %s, namespace: %s", name, namespace))
				}
			}

			if err != nil {
				log.Info(fmt.Sprintf("m.Raw: %+v", m.Raw))
			} else {
				if unstructuredObj.GetKind() == "Deployment" {
					deploymentName := unstructuredObj.GetName()
					nodeSelector, _, _ := unstructured.NestedMap(unstructuredObj.Object, "spec", "template", "spec", "nodeSelector")
					tolerations, _, _ := unstructured.NestedSlice(unstructuredObj.Object, "spec", "template", "spec", "tolerations")
					log.Info(fmt.Sprintf("Manifestwork (new): %s, Deployment Name: %s", name, deploymentName))
					log.Info(fmt.Sprintf("Manifestwork (new): %+v, NodeSelector (before): %+v", name, nodeSelector))
					log.Info(fmt.Sprintf("Manifestwork (new): %+v, Tolerations (before): %+v", name, tolerations))
				}
			}
		}

		found.Spec.Workload.Manifests = manifests
		err = c.Update(context.TODO(), found)
		if err != nil {
			log.Error(err, "Failed to update monitoring-endpoint-monitoring-work work")
			return err
		} else {
			log.Info("manifestwork updated", "namespace", namespace, "name", name)
		}
		return nil
	}

	log.Info("manifestwork already existed/unchanged", "namespace", namespace)
	return nil
}

// generateGlobalManifestResources generates global resources, eg. manifestwork,
// endpoint-metrics-operator deploy and hubInfo Secret...
// this function is expensive and should not be called for each reconcile loop.
func generateGlobalManifestResources(c client.Client, mco *mcov1beta2.MultiClusterObservability) (
	[]workv1.Manifest, *workv1.Manifest, *workv1.Manifest, error) {

	works := []workv1.Manifest{}

	// inject the namespace
	works = injectIntoWork(works, generateNamespace())

	// inject the image pull secret
	if pullSecret == nil {
		var err error
		if pullSecret, err = generatePullSecret(c, config.GetImagePullSecret(mco.Spec)); err != nil {
			return nil, nil, nil, err
		}
	}

	// inject the certificates
	if managedClusterObsCert == nil {
		var err error
		if managedClusterObsCert, err = generateObservabilityServerCACerts(c); err != nil {
			return nil, nil, nil, err
		}
	}
	works = injectIntoWork(works, managedClusterObsCert)

	// generate the metrics allowlist configmap
	if metricsAllowlistConfigMap == nil || ocp311metricsAllowlistConfigMap == nil {
		var err error
		if metricsAllowlistConfigMap, ocp311metricsAllowlistConfigMap, err = generateMetricsListCM(c); err != nil {
			return nil, nil, nil, err
		}
	}

	// inject the alertmanager accessor bearer token secret
	if amAccessorTokenSecret == nil {
		var err error
		if amAccessorTokenSecret, err = generateAmAccessorTokenSecret(c); err != nil {
			return nil, nil, nil, err
		}
	}
	works = injectIntoWork(works, amAccessorTokenSecret)

	// reload resources if empty
	if len(rawExtensionList) == 0 || obsAddonCRDv1 == nil || obsAddonCRDv1beta1 == nil {
		var err error
		rawExtensionList, obsAddonCRDv1, obsAddonCRDv1beta1,
			endpointMetricsOperatorDeploy, imageListConfigMap, err = loadTemplates(mco)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	// inject resouces in templates
	crdv1Work := &workv1.Manifest{RawExtension: runtime.RawExtension{
		Object: obsAddonCRDv1,
	}}
	crdv1beta1Work := &workv1.Manifest{RawExtension: runtime.RawExtension{
		Object: obsAddonCRDv1beta1,
	}}
	for _, raw := range rawExtensionList {
		works = append(works, workv1.Manifest{RawExtension: raw})
	}

	return works, crdv1Work, crdv1beta1Work, nil
}

func createManifestWorks(c client.Client, restMapper meta.RESTMapper,
	clusterNamespace string, clusterName string,
	mco *mcov1beta2.MultiClusterObservability,
	works []workv1.Manifest, allowlist *corev1.ConfigMap,
	crdWork *workv1.Manifest, dep *appsv1.Deployment,
	hubInfo *corev1.Secret, installProm bool) error {

	work := newManifestwork(clusterNamespace+workNameSuffix, clusterNamespace)

	manifests := work.Spec.Workload.Manifests
	// inject observabilityAddon
	obaddon, err := getObservabilityAddon(c, clusterNamespace, mco)
	if err != nil {
		return err
	}
	if obaddon != nil {
		manifests = injectIntoWork(manifests, obaddon)
	}

	manifests = append(manifests, works...)
	manifests = injectIntoWork(manifests, allowlist)

	if clusterName != localClusterName {
		manifests = append(manifests, *crdWork)
	}

	// replace the managedcluster image with the custom registry
	managedClusterImageRegistryMutex.RLock()
	_, hasCustomRegistry := managedClusterImageRegistry[clusterName]
	managedClusterImageRegistryMutex.RUnlock()
	imageRegistryClient := NewImageRegistryClient(c)

	// inject the endpoint operator deployment

	// since we are reusing endpoint operator deployment spec across multiple managed clusters while creating manifestwork,
	// always reset NodeSelector and Tolerations to the default values.
	// Note that this will cause any defaults values set in the deployment template spec to be lost.
	dep.Spec.Template.Spec.NodeSelector = map[string]string{}
	dep.Spec.Template.Spec.Tolerations = []corev1.Toleration{}
	spec := dep.Spec.Template.Spec
	if clusterName == localClusterName {
		spec.NodeSelector = mco.Spec.NodeSelector
		spec.Tolerations = mco.Spec.Tolerations
	}
	log.Info(fmt.Sprintf("Cluster: %+v, Spec.NodeSelector (after): %+v", clusterName, spec.NodeSelector))
	log.Info(fmt.Sprintf("Cluster: %+v, Spec.Tolerations (after): %+v", clusterName, spec.Tolerations))
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

			if hasCustomRegistry {
				oldImage := container.Image
				newImage, err := imageRegistryClient.Cluster(clusterName).ImageOverride(oldImage)
				log.Info("Replace the endpoint operator image", "cluster", clusterName, "newImage", newImage)
				if err == nil {
					spec.Containers[i].Image = newImage
				}
			}
		}
	}
	dep.Spec.Template.Spec = spec
	manifests = injectIntoWork(manifests, dep)
	// replace the pull secret and addon components image
	if hasCustomRegistry {
		log.Info("Replace the default pull secret to custom pull secret", "cluster", clusterName)
		customPullSecret, err := imageRegistryClient.Cluster(clusterName).PullSecret()
		if err == nil && customPullSecret != nil {
			customPullSecret.ResourceVersion = ""
			customPullSecret.Name = config.GetImagePullSecret(mco.Spec)
			customPullSecret.Namespace = spokeNameSpace
			manifests = injectIntoWork(manifests, customPullSecret)
		}

		log.Info("Replace the image list configmap with custom image", "cluster", clusterName)
		newImageListCM := imageListConfigMap.DeepCopy()
		images := newImageListCM.Data
		for key, oldImage := range images {
			newImage, err := imageRegistryClient.Cluster(clusterName).ImageOverride(oldImage)
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
	hubInfo.Data[operatorconfig.ClusterNameKey] = []byte(clusterName)
	manifests = injectIntoWork(manifests, hubInfo)

	work.Spec.Workload.Manifests = manifests

	err = createManifestwork(c, work)
	return err
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
		// and the other stores the servcie account token  with name format (<sa name>-token-<random>),
		// but the service account secrets won't list in the service account any longger.
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
	}

	if tokenSrtName == "" {
		log.Error(
			err,
			"no token secret for Alertmanager accessor serviceaccount",
			"name",
			config.AlertmanagerAccessorSAName,
		)
		return nil, fmt.Errorf(
			"no token secret for Alertmanager accessor serviceaccount: %s",
			config.AlertmanagerAccessorSAName,
		)
	}

	tokenSrt := &corev1.Secret{}
	err = cl.Get(context.TODO(), types.NamespacedName{Name: tokenSrtName,
		Namespace: config.GetDefaultNamespace()}, tokenSrt)
	if err != nil {
		log.Error(err, "Failed to get token secret for Alertmanager accessor serviceaccount", "name", tokenSrtName)
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

// generateObservabilityServerCACerts generates the certificate for managed cluster
func generateObservabilityServerCACerts(client client.Client) (*corev1.Secret, error) {
	ca := &corev1.Secret{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: config.ServerCACerts,
		Namespace: config.GetDefaultNamespace()}, ca)
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
func generateMetricsListCM(client client.Client) (*corev1.ConfigMap, *corev1.ConfigMap, error) {
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

	ocp311AllowlistCM := metricsAllowlistCM.DeepCopy()

	allowlist, ocp3Allowlist, err := getAllowList(client, operatorconfig.AllowlistConfigMapName)
	if err != nil {
		log.Error(err, "Failed to get metrics allowlist configmap "+operatorconfig.AllowlistConfigMapName)
		return nil, nil, err
	}

	customAllowlist, _, err := getAllowList(client, config.AllowlistCustomConfigMapName)
	if err == nil {
		allowlist.NameList = mergeMetrics(allowlist.NameList, customAllowlist.NameList)
		allowlist.MatchList = mergeMetrics(allowlist.MatchList, customAllowlist.MatchList)
		allowlist.CollectRuleGroupList = mergeCollectorRuleGroupList(allowlist.CollectRuleGroupList, customAllowlist.CollectRuleGroupList)
		if customAllowlist.RecordingRuleList != nil {
			allowlist.RecordingRuleList = append(allowlist.RecordingRuleList, customAllowlist.RecordingRuleList...)
		} else {
			//check if rules are specified for backward compatibility
			allowlist.RecordingRuleList = append(allowlist.RecordingRuleList, customAllowlist.RuleList...)
		}
		for k, v := range customAllowlist.RenameMap {
			allowlist.RenameMap[k] = v
		}
		ocp3Allowlist.NameList = mergeMetrics(ocp3Allowlist.NameList, customAllowlist.NameList)
		ocp3Allowlist.MatchList = mergeMetrics(ocp3Allowlist.MatchList, customAllowlist.MatchList)
		ocp3Allowlist.RuleList = append(ocp3Allowlist.RuleList, customAllowlist.RuleList...)
		for k, v := range customAllowlist.RenameMap {
			ocp3Allowlist.RenameMap[k] = v
		}
	} else {
		log.Info("There is no custom metrics allowlist configmap in the cluster")
	}

	data, err := yaml.Marshal(allowlist)
	if err != nil {
		log.Error(err, "Failed to marshal allowlist data")
		return nil, nil, err
	}
	metricsAllowlistCM.Data["metrics_list.yaml"] = string(data)
	data, err = yaml.Marshal(ocp3Allowlist)
	if err != nil {
		log.Error(err, "Failed to marshal allowlist data")
		return nil, nil, err
	}
	ocp311AllowlistCM.Data["ocp311_metrics_list.yaml"] = string(data)
	return metricsAllowlistCM, ocp311AllowlistCM, nil
}

func getAllowList(client client.Client, name string) (*operatorconfig.MetricsAllowlist, *operatorconfig.MetricsAllowlist, error) {
	found := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      name,
		Namespace: config.GetDefaultNamespace(),
	}
	err := client.Get(context.TODO(), namespacedName, found)
	if err != nil {
		return nil, nil, err
	}
	allowlist := &operatorconfig.MetricsAllowlist{}
	err = yaml.Unmarshal([]byte(found.Data["metrics_list.yaml"]), allowlist)
	if err != nil {
		log.Error(err, "Failed to unmarshal metrics_list.yaml data in configmap "+name)
		return nil, nil, err
	}
	ocp3Allowlist := &operatorconfig.MetricsAllowlist{}
	err = yaml.Unmarshal([]byte(found.Data["ocp311_metrics_list.yaml"]), ocp3Allowlist)
	if err != nil {
		log.Error(err, "Failed to unmarshal ocp311_metrics_list data in configmap "+name)
		return nil, nil, err
	}
	return allowlist, ocp3Allowlist, nil
}

func mergeMetrics(defaultAllowlist []string, customAllowlist []string) []string {
	customMetrics := []string{}
	deletedMetrics := map[string]bool{}
	for _, name := range customAllowlist {
		if !strings.HasPrefix(name, "-") {
			customMetrics = append(customMetrics, name)
		} else {
			deletedMetrics[strings.TrimPrefix(name, "-")] = true
		}
	}

	metricsRecorder := map[string]bool{}
	mergedMetrics := []string{}
	defaultAllowlist = append(defaultAllowlist, customMetrics...)
	for _, name := range defaultAllowlist {
		if metricsRecorder[name] {
			continue
		}

		if !deletedMetrics[name] {
			mergedMetrics = append(mergedMetrics, name)
			metricsRecorder[name] = true
		}
	}

	return mergedMetrics
}

func mergeCollectorRuleGroupList(defaultCollectRuleGroupList []operatorconfig.CollectRuleGroup, customCollectRuleGroupList []operatorconfig.CollectRuleGroup) []operatorconfig.CollectRuleGroup {
	deletedCollectRuleGroups := map[string]bool{}
	mergedCollectRuleGroups := []operatorconfig.CollectRuleGroup{}

	for _, collectRuleGroup := range customCollectRuleGroupList {
		if strings.HasPrefix(collectRuleGroup.Name, "-") {
			deletedCollectRuleGroups[strings.TrimPrefix(collectRuleGroup.Name, "-")] = true
		} else {
			mergedCollectRuleGroups = append(mergedCollectRuleGroups, collectRuleGroup)
		}
	}

	for _, collectRuleGroup := range defaultCollectRuleGroupList {
		if !deletedCollectRuleGroups[collectRuleGroup.Name] {
			mergedCollectRuleGroups = append(mergedCollectRuleGroups, collectRuleGroup)
		}
	}

	return mergedCollectRuleGroups
}

func getObservabilityAddon(c client.Client, namespace string,
	mco *mcov1beta2.MultiClusterObservability) (*mcov1beta1.ObservabilityAddon, error) {
	found := &mcov1beta1.ObservabilityAddon{}
	namespacedName := types.NamespacedName{
		Name:      obsAddonName,
		Namespace: namespace,
	}
	err := c.Get(context.TODO(), namespacedName, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		log.Error(err, "Failed to check observabilityAddon")
		return nil, err
	}
	if found.ObjectMeta.DeletionTimestamp != nil {
		return nil, nil
	}

	return &mcov1beta1.ObservabilityAddon{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "observability.open-cluster-management.io/v1beta1",
			Kind:       "ObservabilityAddon",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAddonName,
			Namespace: spokeNameSpace,
		},
		Spec: mcoshared.ObservabilityAddonSpec{
			EnableMetrics: mco.Spec.ObservabilityAddonSpec.EnableMetrics,
			Interval:      mco.Spec.ObservabilityAddonSpec.Interval,
			Resources:     config.GetOBAResources(mco.Spec.ObservabilityAddonSpec),
		},
	}, nil
}

func removeObservabilityAddon(client client.Client, namespace string) error {
	name := namespace + workNameSuffix
	found := &workv1.ManifestWork{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to check manifestwork", "namespace", namespace, "name", name)
		return err
	}

	obj, err := util.GetObject(found.Spec.Workload.Manifests[0].RawExtension)
	if err != nil {
		return err
	}
	if obj.GetObjectKind().GroupVersionKind().Kind == "ObservabilityAddon" {
		updateManifests := found.Spec.Workload.Manifests[1:]
		found.Spec.Workload.Manifests = updateManifests

		err = client.Update(context.TODO(), found)
		if err != nil {
			log.Error(err, "Failed to update manifestwork", "namespace", namespace, "name", name)
			return err
		}
	}
	return nil
}

func logSizeErrorDetails(str string, work *workv1.ManifestWork) {
	if strings.Contains(str, "the size of manifests") {
		var keyVal []interface{}
		for _, manifest := range work.Spec.Workload.Manifests {
			raw, _ := json.Marshal(manifest.RawExtension.Object)
			keyVal = append(keyVal, "kind", manifest.RawExtension.Object.GetObjectKind().
				GroupVersionKind().Kind, "size", len(raw))
		}
		log.Info("size of manifest", keyVal...)
	}
}
