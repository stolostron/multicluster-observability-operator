// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workv1 "github.com/open-cluster-management/api/work/v1"
	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	workName = "monitoring-endpoint-monitoring-work"
)

func deleteManifestWork(client client.Client, namespace string) error {
	found := &workv1.ManifestWork{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
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

func createManifestWork(client client.Client, clusterNamespace string,
	clusterName string,
	mco *mcov1beta1.MultiClusterObservability,
	imagePullSecret *corev1.Secret) error {

	secret, err := createKubeSecret(client, clusterNamespace)
	if err != nil {
		return err
	}
	work := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workName,
			Namespace: clusterNamespace,
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{
						runtime.RawExtension{
							Object: createNameSpace(),
						},
					},
					{
						runtime.RawExtension{
							Object: secret,
						},
					},
				},
			},
		},
	}
	templates, err := loadTemplates(clusterNamespace, mco)
	if err != nil {
		log.Error(err, "Failed to load templates")
		return err
	}
	manifests := work.Spec.Workload.Manifests
	for _, raw := range templates {
		manifests = append(manifests, workv1.Manifest{raw})
	}

	//create image pull secret
	manifests = append(manifests,
		workv1.Manifest{
			runtime.RawExtension{
				Object: &corev1.Secret{
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
				},
			},
		})

	// create the hub info secret
	hubInfo, err := newHubInfoSecret(client, mco.Namespace, spokeNameSpace, clusterName)
	if err != nil {
		return err
	}
	manifests = append(manifests, workv1.Manifest{
		runtime.RawExtension{
			Object: hubInfo,
		},
	})

	work.Spec.Workload.Manifests = manifests

	found := &workv1.ManifestWork{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: clusterNamespace}, found)
	if err != nil && errors.IsNotFound(err) {
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
