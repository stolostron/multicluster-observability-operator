// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	certificatesv1 "k8s.io/api/certificates/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcoshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/certificates"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	workNameSuffix             = "-observability"
	localClusterName           = "local-cluster"
	workPostponeDeleteAnnoKey  = "open-cluster-management/postpone-delete"
	hubEndpointOperatorName    = "endpoint-observability-operator"
	hubMetricsCollectorName    = "metrics-collector-deployment"
	hubUwlMetricsCollectorName = "uwl-metrics-collector-deployment"
	hubUwlMetricsCollectorNs   = "openshift-user-workload-monitoring"
)

// intermediate resources for the manifest work.
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
	hubManifestCopy  []workv1.Manifest
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
		log.Info("Updating manifestwork", namespace, namespace, "name", name)
		found.Spec.Workload.Manifests = manifests
		err = c.Update(context.TODO(), found)
		if err != nil {
			log.Error(err, "Failed to update monitoring-endpoint-monitoring-work work")
			logSizeErrorDetails(fmt.Sprint(err), work)
			return err
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

func createManifestWorks(
	c client.Client,
	clusterNamespace string,
	clusterName string,
	mco *mcov1beta2.MultiClusterObservability,
	works []workv1.Manifest,
	allowlist *corev1.ConfigMap,
	crdWork *workv1.Manifest,
	dep *appsv1.Deployment,
	hubInfo *corev1.Secret,
	addonConfig *addonv1alpha1.AddOnDeploymentConfig,
	installProm bool,
) error {
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
	spec := dep.Spec.Template.Spec
	if addonConfig.Spec.NodePlacement != nil {
		spec.NodeSelector = addonConfig.Spec.NodePlacement.NodeSelector
		spec.Tolerations = addonConfig.Spec.NodePlacement.Tolerations
	} else if clusterName == localClusterName {
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
			if clusterName != localClusterName {
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
						//CA is allowed only when HTTPS proxy is set
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
				newImage, err := imageRegistryClient.Cluster(clusterName).ImageOverride(oldImage)
				log.Info("Replace the endpoint operator image", "cluster", clusterName, "newImage", newImage)
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

	log.Info(fmt.Sprintf("Cluster: %+v, Spec.NodeSelector (after): %+v", clusterName, spec.NodeSelector))
	log.Info(fmt.Sprintf("Cluster: %+v, Spec.Tolerations (after): %+v", clusterName, spec.Tolerations))

	if clusterName != clusterNamespace {
		spec.Volumes = []corev1.Volume{}
		spec.Containers[0].VolumeMounts = []corev1.VolumeMount{}
		for i, env := range spec.Containers[0].Env {
			if env.Name == "HUB_KUBECONFIG" {
				spec.Containers[0].Env[i].Value = ""
				break
			}
		}
		//Set HUB_ENDPOINT_OPERATOR when the endpoint operator is installed in hub cluster
		spec.Containers[0].Env = append(spec.Containers[0].Env, corev1.EnvVar{
			Name:  "HUB_ENDPOINT_OPERATOR",
			Value: "true",
		})

		dep.ObjectMeta.Name = hubEndpointOperatorName
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

	if clusterName != clusterNamespace && os.Getenv("UNIT_TEST") != "true" {
		// ACM 8509: Special case for hub/local cluster metrics collection
		// install the endpoint operator into open-cluster-management-observability namespace for the hub cluster
		log.Info("Creating resource for hub metrics collection", "cluster", clusterName)
		err = createUpdateResourcesForHubMetricsCollection(c, manifests)
	} else {
		err = createManifestwork(c, work)
	}

	return err
}

func createCSR() ([]byte, []byte) {
	keys, _ := rsa.GenerateKey(rand.Reader, 2048)

	oidOrganization := []int{2, 5, 4, 11} // Object Identifier (OID) for Organization Unit
	oidUser := []int{2, 5, 4, 3}          // Object Identifier (OID) for User

	var csrTemplate = x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"Red Hat, Inc."},
			Country:      []string{"US"},
			CommonName:   operatorconfig.ClientCACertificateCN,
			ExtraNames: []pkix.AttributeTypeAndValue{
				{Type: oidOrganization, Value: "acm"},
				{Type: oidUser, Value: "managed-cluster-observability"},
			},
		},
		DNSNames:           []string{"observability-controller.addon.open-cluster-management.io"},
		SignatureAlgorithm: x509.SHA512WithRSA,
	}
	csrCertificate, _ := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, keys)
	csr := pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE REQUEST", Bytes: csrCertificate,
	})

	privateKey := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(keys),
	})

	return csr, privateKey
}

func createMtlsCertSecretForHubCollector() (*corev1.Secret, error) {
	csrBytes, privateKeyBytes := createCSR()
	csr := &certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request: csrBytes,
			Usages:  []certificatesv1.KeyUsage{certificatesv1.UsageDigitalSignature, certificatesv1.UsageClientAuth},
		},
	}
	signedClientCert := certificates.Sign(csr)
	if signedClientCert == nil {
		log.Error(nil, "failed to sign CSR")
		return nil, errors.New("failed to sign CSR")
	} else {
		//Create a secret
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorconfig.MtlsCertName,
				Namespace: config.GetDefaultNamespace(),
			},
			Data: map[string][]byte{
				"tls.crt": signedClientCert,
				"tls.key": privateKeyBytes,
			},
		}, nil
	}
}

func createUpdateResourcesForHubMetricsCollection(c client.Client, manifests []workv1.Manifest) error {
	//create csr for hub metrics collection
	hubMtlsSecret, err := createMtlsCertSecretForHubCollector()
	if err != nil {
		log.Error(err, "Failed to create client cert secret for hub metrics collection")
		return err
	}
	manifests = injectIntoWork(manifests, hubMtlsSecret)

	//Make a deep copy of all the manifests since there are some global resources that can be updated due to this function
	hubManifestCopy = make([]workv1.Manifest, len(manifests))
	for i, manifest := range manifests {
		obj := manifest.RawExtension.Object.DeepCopyObject()
		hubManifestCopy[i] = workv1.Manifest{RawExtension: runtime.RawExtension{Object: obj}}
		hubManifestCopy[i] = workv1.Manifest{RawExtension: runtime.RawExtension{Object: obj}}
	}
	for _, manifest := range hubManifestCopy {
		obj := manifest.RawExtension.Object.(client.Object)
		if obj.GetObjectKind().GroupVersionKind().Kind == "Namespace" || obj.GetObjectKind().GroupVersionKind().Kind == "ObservabilityAddon" {
			// We do not want to create ObservabilityAddon and namespace open-cluster-management-add-on observability for hub cluster
			continue
		}
		kind := obj.GetObjectKind().GroupVersionKind().Kind
		if kind != "ClusterRole" && kind != "ClusterRoleBinding" && kind != "CustomResourceDefinition" {
			obj.SetNamespace(config.GetDefaultNamespace())
		}
		if obj.GetObjectKind().GroupVersionKind().Kind == "ClusterRoleBinding" {
			role := obj.(*rbacv1.ClusterRoleBinding)
			role.Subjects[0].Namespace = config.GetDefaultNamespace()
		}
		err := c.Create(context.TODO(), obj)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			log.Error(err, "Failed to create resource", "kind", obj.GetObjectKind().GroupVersionKind().Kind)
			return err
		}
	}
	return nil
}

// Delete resources created for hub metrics collection
func DeleteHubMetricsCollectionDeployments(c client.Client) error {
	log.Info("Coleen Deleting resources for hub metrics collection")
	for _, manifest := range hubManifestCopy {
		obj := manifest.RawExtension.Object.(client.Object)
		log.Info("Coleen Deleting resource", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName(), "namespace", obj.GetNamespace())

		err := c.Delete(context.TODO(), obj)
		if err != nil && !k8serrors.IsNotFound(err) {
			log.Error(err, "Failed to delete resource", "kind", obj.GetObjectKind().GroupVersionKind().Kind)
			return err
		}
	}
	for _, name := range []string{hubMetricsCollectorName, hubUwlMetricsCollectorName} {
		err := c.Delete(context.TODO(), &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: config.GetDefaultNamespace(),
			},
		})
		if err != nil && !k8serrors.IsNotFound(err) {
			log.Error(err, "Failed to delete hub metrics-collector deployment")
			return err

		}
	}
	err := RevertHubClusterMonitoringConfig(context.TODO(), c)
	if err != nil {
		log.Error(err, "Failed to revert cluster monitoring config")
		return err
	}
	err = DeleteHubMonitoringClusterRoleBinding(context.TODO(), c)
	if err != nil {
		log.Error(err, "Failed to delete monitoring cluster role binding for hub metrics collection")
		return err
	}
	err = DeleteHubCAConfigmap(context.TODO(), c)
	if err != nil {
		log.Error(err, "Failed to delete CA configmap for hub metrics collection")
		return err
	}
	isHypershift := true
	if os.Getenv("UNIT_TEST") != "true" {
		crdClient, err := util.GetOrCreateCRDClient()
		if err != nil {
			log.Error(err, "Failed to create CRD client")
			return err
		}
		isHypershift, err = util.CheckCRDExist(crdClient, "hostedclusters.hypershift.openshift.io")
		if err != nil {
			log.Error(err, "Failed to check if the CRD hostedclusters.hypershift.openshift.io exists")
			return err
		}
	}
	if isHypershift {
		err = DeleteServiceMonitors(context.TODO(), c)
		if err != nil {
			log.Error(err, "Failed to delete service monitors for hub metrics collection")
			return err
		}
	}
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

	allowlist, ocp3Allowlist, uwlAllowlist, err := util.GetAllowList(client,
		operatorconfig.AllowlistConfigMapName, config.GetDefaultNamespace())
	if err != nil {
		log.Error(err, "Failed to get metrics allowlist configmap "+operatorconfig.AllowlistConfigMapName)
		return nil, nil, err
	}

	customAllowlist, _, customUwlAllowlist, err := util.GetAllowList(client,
		config.AllowlistCustomConfigMapName, config.GetDefaultNamespace())
	if err == nil {
		allowlist, ocp3Allowlist, uwlAllowlist = util.MergeAllowlist(allowlist,
			customAllowlist, ocp3Allowlist, uwlAllowlist, customUwlAllowlist)
	} else {
		log.Info("There is no custom metrics allowlist configmap in the cluster")
	}

	data, err := yaml.Marshal(allowlist)
	if err != nil {
		log.Error(err, "Failed to marshal allowlist data")
		return nil, nil, err
	}
	uwlData, err := yaml.Marshal(uwlAllowlist)
	if err != nil {
		log.Error(err, "Failed to marshal allowlist uwlAllowlist")
		return nil, nil, err
	}
	metricsAllowlistCM.Data[operatorconfig.MetricsConfigMapKey] = string(data)
	metricsAllowlistCM.Data[operatorconfig.UwlMetricsConfigMapKey] = string(uwlData)

	data, err = yaml.Marshal(ocp3Allowlist)
	if err != nil {
		log.Error(err, "Failed to marshal allowlist data")
		return nil, nil, err
	}
	ocp311AllowlistCM.Data[operatorconfig.MetricsOcp311ConfigMapKey] = string(data)
	return metricsAllowlistCM, ocp311AllowlistCM, nil
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

	if namespace == config.GetDefaultNamespace() {
		return &mcov1beta1.ObservabilityAddon{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "observability.open-cluster-management.io/v1beta1",
				Kind:       "ObservabilityAddon",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      obsAddonName,
				Namespace: config.GetDefaultNamespace(),
			},
			Spec: mcoshared.ObservabilityAddonSpec{
				EnableMetrics: mco.Spec.ObservabilityAddonSpec.EnableMetrics,
				Interval:      mco.Spec.ObservabilityAddonSpec.Interval,
				Resources:     config.GetOBAResources(mco.Spec.ObservabilityAddonSpec),
			},
		}, nil
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
