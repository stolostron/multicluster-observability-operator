// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	imageregistryv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/imageregistry/v1alpha1"
	"github.com/open-cluster-management/multicloud-operators-foundation/pkg/helpers/imageregistry"
)

var (
	managedClusterImageRegistry = map[string]string{}
)

func updateManagedClusterImageRegistry(obj client.Object) {
	if imageReg, ok := obj.GetLabels()[imageregistryv1alpha1.ClusterImageRegistryLabel]; ok {
		managedClusterImageRegistry[obj.GetName()] = imageReg
	}
}

func NewImageRegistryClient(c client.Client) imageregistry.Client {
	return imageregistry.NewDefaultClient(c)
}
