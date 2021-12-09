// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package placementrule

import (
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	imageregistryv1alpha1 "github.com/open-cluster-management/multicloud-operators-foundation/pkg/apis/imageregistry/v1alpha1"
	"github.com/open-cluster-management/multicloud-operators-foundation/pkg/helpers/imageregistry"
)

var (
	managedClusterImageRegistry      = map[string]string{}
	managedClusterImageRegistryMutex = &sync.RWMutex{}
)

func updateManagedClusterImageRegistry(obj client.Object) {
	if imageReg, ok := obj.GetLabels()[imageregistryv1alpha1.ClusterImageRegistryLabel]; ok {
		managedClusterImageRegistryMutex.Lock()
		managedClusterImageRegistry[obj.GetName()] = imageReg
		managedClusterImageRegistryMutex.Unlock()
	}
}

func NewImageRegistryClient(c client.Client) imageregistry.Client {
	return imageregistry.NewDefaultClient(c)
}
