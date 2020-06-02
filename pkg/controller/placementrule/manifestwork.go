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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workv1 "github.com/open-cluster-management/api/work/v1"
	placev1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

const (
	workName = "monitoring-endpoint-metrics-work"
)

func createManifestWork(client client.Client,
	p *placev1.PlacementRule, scheme *runtime.Scheme,
	namespace string,
	mcm *monitoringv1alpha1.MultiClusterMonitoring,
	imagePullSecret *corev1.Secret) error {
	found := &workv1.ManifestWork{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: workName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating monitoring-endpoint-metrics-work work", "namespace", namespace)
		secret, err := createKubeSecret(client, namespace)
		if err != nil {
			return err
		}
		work := &workv1.ManifestWork{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workName,
				Namespace: namespace,
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

		// Set PlacementRule instance as the owner and controller
		if err := controllerutil.SetControllerReference(p, work, scheme); err != nil {
			return err
		}

		templates, err := loadTemplates(namespace, mcm)
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

		work.Spec.Workload.Manifests = manifests
		err = client.Create(context.TODO(), work)
		if err != nil {
			log.Error(err, "Failed to create monitoring-endpoint-metrics-work work")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check monitoring-endpoint-metrics-work work")
		return err
	}
	log.Info("manifestwork already existed", "namespace", namespace)
	return nil
}
