// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workv1 "github.com/open-cluster-management/api/work/v1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/controller/multiclusterobservability"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	workName      = "endpoint-observability-work"
	configMapName = "observability-metrics-whitelist"
)

var (
	metricsWhitelist = &corev1.ConfigMap{}
)

func deleteManifestWork(client client.Client, namespace string) error {
	err := deleteRes(client, namespace)
	if err != nil {
		return err
	}

	found := &workv1.ManifestWork{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to check monitoring-endpoint-monitoring-work work", "namespace", namespace)
		return err
	}
	err = client.Delete(context.TODO(), found)
	if err != nil {
		log.Error(err, "Failed to delete monitoring-endpoint-monitoring-work work", "namespace", namespace)
	}
	log.Info("manifestwork is deleted", "namespace", namespace)
	return err
}

func injectIntoWork(works []workv1.Manifest, obj runtime.Object) []workv1.Manifest {
	works = append(works,
		workv1.Manifest{
			runtime.RawExtension{
				Object: obj,
			},
		})
	return works
}

func createManifestWork(client client.Client, restMapper meta.RESTMapper,
	clusterNamespace string, clusterName string,
	mco *mcov1beta1.MultiClusterObservability,
	imagePullSecret *corev1.Secret) error {

	work := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workName,
			Namespace: clusterNamespace,
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

	// inject resouces in templates
	templates, err := loadTemplates(clusterNamespace, mco)
	if err != nil {
		log.Error(err, "Failed to load templates")
		return err
	}
	manifests := work.Spec.Workload.Manifests
	for _, raw := range templates {
		manifests = append(manifests, workv1.Manifest{raw})
	}

	// inject namespace
	manifests = injectIntoWork(manifests, createNameSpace())

	// inject kube secret
	secret, err := createKubeSecret(client, restMapper, clusterNamespace)
	if err != nil {
		return err
	}
	manifests = injectIntoWork(manifests, secret)

	//create image pull secret
	if imagePullSecret != nil {
		pull := getPullSecret(imagePullSecret)
		manifests = injectIntoWork(manifests, pull)
	}

	// inject the hub info secret
	hubInfo, err := newHubInfoSecret(client, mco.Namespace, spokeNameSpace, clusterName)
	if err != nil {
		return err
	}
	manifests = injectIntoWork(manifests, hubInfo)

	// inject the certificates
	certs, err := getCerts(client, clusterNamespace)
	if err != nil {
		return err
	}
	manifests = injectIntoWork(manifests, certs)

	// inject the metrics whitelist configmap
	mList, err := getMetricsListCM(client)
	if err != nil {
		return err
	}
	manifests = injectIntoWork(manifests, mList)

	work.Spec.Workload.Manifests = manifests

	found := &workv1.ManifestWork{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: clusterNamespace}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		log.Info("Creating monitoring-endpoint-monitoring-work work", "namespace", clusterNamespace)

		err = client.Create(context.TODO(), work)
		if err != nil {
			log.Error(err, "Failed to create monitoring-endpoint-monitoring-work work")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check monitoring-endpoint-monitoring-work work")
		return err
	}

	if found.GetDeletionTimestamp() != nil {
		log.Error(err, "Existing manifestwork is terminating, skip and reconcile later")
		return errors.New("Existing manifestwork is terminating, skip and reconcile later")
	}

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
		log.Info("Reverting monitoring-endpoint-monitoring-work work", "namespace", clusterNamespace)
		work.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(context.TODO(), work)
		if err != nil {
			log.Error(err, "Failed to update monitoring-endpoint-monitoring-work work")
			return err
		}
		return nil
	}

	log.Info("manifestwork already existed/unchanged", "namespace", clusterNamespace)
	return nil
}

func getPullSecret(imagePullSecret *corev1.Secret) *corev1.Secret {
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
	}
}

func getCerts(client client.Client, namespace string) (*corev1.Secret, error) {

	ca := &corev1.Secret{}
	caName := multiclusterobservability.GetServerCerts()
	err := client.Get(context.TODO(), types.NamespacedName{Name: caName,
		Namespace: config.GetDefaultNamespace()}, ca)
	if err != nil {
		log.Error(err, "Failed to get ca cert secret", "name", caName)
		return nil, err
	}

	certs := &corev1.Secret{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: certsName, Namespace: namespace}, certs)
	if err != nil {
		log.Error(err, "Failed to get certs secret", "name", certsName, "namespace", namespace)
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
			"ca.crt":  ca.Data["ca.crt"],
			"tls.crt": certs.Data["tls.crt"],
			"tls.key": certs.Data["tls.key"],
		},
	}, nil
}

func getMetricsListCM(client client.Client) (*corev1.ConfigMap, error) {
	if metricsWhitelist.Name == "" {
		metricsWhitelist = &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: spokeNameSpace,
			},
		}

		found := &corev1.ConfigMap{}
		namespacedName := types.NamespacedName{
			Name:      configMapName,
			Namespace: config.GetDefaultNamespace(),
		}
		err := client.Get(context.TODO(), namespacedName, found)
		if err != nil {
			log.Error(err, "Failed to get metrics whitelist configmap")
			return nil, err
		}
		metricsWhitelist.Data = found.Data
	}
	return metricsWhitelist, nil
}
