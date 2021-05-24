// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workv1 "github.com/open-cluster-management/api/work/v1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta1"
	mcov1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
)

const (
	workNameSuffix   = "-observability"
	localClusterName = "local-cluster"
)

type MetricsAllowlist struct {
	NameList  []string          `yaml:"names"`
	MatchList []string          `yaml:"matches"`
	ReNameMap map[string]string `yaml:"renames"`
	RuleList  []Rule            `yaml:"rules"`
}

// Rule is the struct for recording rules and alert rules
type Rule struct {
	Record string `yaml:"record"`
	Expr   string `yaml:"expr"`
}

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
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{},
			},
		},
	}
}

func createManifestwork(c client.Client, work *workv1.ManifestWork) error {
	name := work.ObjectMeta.Name
	namespace := work.ObjectMeta.Namespace
	found := &workv1.ManifestWork{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		log.Info("Creating manifestwork", "namespace", namespace, "name", name)

		err = c.Create(context.TODO(), work)
		if err != nil {
			log.Error(err, "Failed to create manifestwork", "namespace", namespace, "name", name)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check manifestwork", namespace, "name", name)
		return err
	}

	if found.GetDeletionTimestamp() != nil {
		log.Info("Existing manifestwork is terminating, skip and reconcile later")
		return errors.New("Existing manifestwork is terminating, skip and reconcile later")
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
		log.Info("Updating manifestwork", namespace, namespace, "name", name)
		work.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = c.Update(context.TODO(), work)
		if err != nil {
			log.Error(err, "Failed to update monitoring-endpoint-monitoring-work work")
			return err
		}
		return nil
	}

	log.Info("manifestwork already existed/unchanged", "namespace", namespace)
	return nil
}

func getGlobalManifestResources(c client.Client, mco *mcov1beta2.MultiClusterObservability) (
	[]workv1.Manifest, *workv1.Manifest, *appsv1.Deployment, *corev1.Secret, error) {

	works := []workv1.Manifest{}

	hubInfo, err := newHubInfoSecret(c, config.GetDefaultNamespace(), spokeNameSpace, mco)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// inject namespace
	works = injectIntoWork(works, createNameSpace())

	//create image pull secret
	pull, err := getPullSecret(c, config.GetImagePullSecret(mco.Spec))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if pull != nil {
		works = injectIntoWork(works, pull)
	}

	// inject the certificates
	certs, err := getCerts(c)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	works = injectIntoWork(works, certs)

	// inject the metrics allowlist configmap
	mList, err := getMetricsListCM(c)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	works = injectIntoWork(works, mList)

	// inject the alertmanager accessor bearer token secret
	amAccessorTokenSecret, err := getAmAccessorTokenSecret(c)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	works = injectIntoWork(works, amAccessorTokenSecret)

	// inject resouces in templates
	templates, crd, dep, err := loadTemplates(mco)
	if err != nil {
		log.Error(err, "Failed to load templates")
		return nil, nil, nil, nil, err
	}
	crdWork := &workv1.Manifest{RawExtension: runtime.RawExtension{
		Object: crd,
	}}
	for _, raw := range templates {
		works = append(works, workv1.Manifest{RawExtension: raw})
	}

	return works, crdWork, dep, hubInfo, nil
}

func createManifestWorks(c client.Client, restMapper meta.RESTMapper,
	clusterNamespace string, clusterName string,
	mco *mcov1beta2.MultiClusterObservability,
	works []workv1.Manifest, crdWork *workv1.Manifest, dep *appsv1.Deployment, hubInfo *corev1.Secret) error {

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

	if clusterName != localClusterName {
		manifests = append(manifests, *crdWork)
	}

	// inject the endpoint operator deployment
	spec := dep.Spec.Template.Spec
	for _, container := range spec.Containers {
		if container.Name == "endpoint-observability-operator" {
			for j, env := range container.Env {
				if env.Name == "HUB_NAMESPACE" {
					container.Env[j].Value = clusterNamespace
				}
			}
		}
	}
	manifests = injectIntoWork(manifests, dep)

	// inject the hub info secret
	hubInfo.Data["clusterName"] = []byte(clusterName)
	manifests = injectIntoWork(manifests, hubInfo)

	work.Spec.Workload.Manifests = manifests

	err = createManifestwork(c, work)
	return err
}

func getAmAccessorTokenSecret(client client.Client) (*corev1.Secret, error) {
	amAccessorSA := &corev1.ServiceAccount{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: config.AlertmanagerAccessorSAName,
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
		log.Error(err, "no token secret for Alertmanager accessor serviceaccount", "name", config.AlertmanagerAccessorSAName)
		return nil, fmt.Errorf("no token secret for Alertmanager accessor serviceaccount: %s", config.AlertmanagerAccessorSAName)
	}

	tokenSrt := &corev1.Secret{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: tokenSrtName,
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

func getPullSecret(c client.Client, name string) (*corev1.Secret, error) {
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

func getCerts(client client.Client) (*corev1.Secret, error) {

	ca := &corev1.Secret{}
	caName := config.ServerCACerts
	err := client.Get(context.TODO(), types.NamespacedName{Name: caName,
		Namespace: config.GetDefaultNamespace()}, ca)
	if err != nil {
		log.Error(err, "Failed to get ca cert secret", "name", caName)
		return nil, err
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      certsName,
			Namespace: spokeNameSpace,
		},
		Data: map[string][]byte{
			"ca.crt": ca.Data["tls.crt"],
		},
	}, nil
}

func getMetricsListCM(client client.Client) (*corev1.ConfigMap, error) {
	metricsAllowlist := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AllowlistConfigMapName,
			Namespace: spokeNameSpace,
		},
		Data: map[string]string{},
	}

	allowlist, err := getAllowList(client, config.AllowlistConfigMapName)
	if err != nil {
		log.Error(err, "Failed to get metrics allowlist configmap "+config.AllowlistConfigMapName)
		return nil, err
	}

	customAllowlist, err := getAllowList(client, config.AllowlistCustomConfigMapName)
	if err == nil {
		allowlist.NameList = mergeMetrics(allowlist.NameList, customAllowlist.NameList)
		allowlist.MatchList = mergeMetrics(allowlist.MatchList, customAllowlist.MatchList)
		allowlist.RuleList = append(allowlist.RuleList, customAllowlist.RuleList...)
		for k, v := range customAllowlist.ReNameMap {
			allowlist.ReNameMap[k] = v
		}
	} else {
		log.Info("There is no custom metrics allowlist configmap in the cluster")
	}

	data, err := yaml.Marshal(allowlist)
	if err != nil {
		log.Error(err, "Failed to marshal allowlist data")
		return nil, err
	}
	metricsAllowlist.Data["metrics_list.yaml"] = string(data)
	return metricsAllowlist, nil
}

func getAllowList(client client.Client, name string) (*MetricsAllowlist, error) {
	found := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      name,
		Namespace: config.GetDefaultNamespace(),
	}
	err := client.Get(context.TODO(), namespacedName, found)
	if err != nil {
		return nil, err
	}
	allowlist := &MetricsAllowlist{}
	err = yaml.Unmarshal([]byte(found.Data["metrics_list.yaml"]), allowlist)
	if err != nil {
		log.Error(err, "Failed to unmarshal data in configmap "+name)
		return nil, err
	}
	return allowlist, nil
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
		Spec: *mco.Spec.ObservabilityAddonSpec,
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
