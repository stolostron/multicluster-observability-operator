// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	workv1 "github.com/open-cluster-management/api/work/v1"
)

const (
	workName = "monitoring-endpoint-metrics-work"
)

func createManifestWork(client client.Client, namespace string) error {
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
		templates, err := loadTemplates()
		if err != nil {
			log.Error(err, "Failed to load templates")
			return err
		}
		manifests := work.Spec.Workload.Manifests
		for _, raw := range templates {
			manifests = append(manifests, workv1.Manifest{raw})
		}
		work.Spec.Workload.Manifests = manifests
		rYaml, err := yaml.Marshal(work)
		if err != nil {
			return err
		}
		log.Info("Debug", "work", rYaml)
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
