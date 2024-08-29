// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ClusterImageRegistriesAnnotation value is a json string of ImageRegistries.
	ClusterImageRegistriesAnnotation = "open-cluster-management.io/image-registries"
)

type Registry struct {
	// Mirror is the mirrored registry of the Source. Will be ignored if Mirror is empty.
	Mirror string `json:"mirror"`

	// Source is the source registry. All image registries will be replaced by Mirror if Source is empty.
	Source string `json:"source"`
}

// ImageRegistries is value of the image registries annotation includes the mirror and source registries.
// The source registry will be replaced by the Mirror.
// The larger index will work if the Sources are the same.
type ImageRegistries struct {
	PullSecret string     `json:"pullSecret"`
	Registries []Registry `json:"registries"`
}

var (
	managedClusterImageRegistry      = map[string]string{}
	managedClusterImageRegistryMutex = &sync.RWMutex{}
)

func updateManagedClusterImageRegistry(obj client.Object) {
	if imageReg, ok := obj.GetAnnotations()[ClusterImageRegistriesAnnotation]; ok {
		managedClusterImageRegistryMutex.Lock()
		managedClusterImageRegistry[obj.GetName()] = imageReg
		managedClusterImageRegistryMutex.Unlock()
	}
}

func NewImageRegistryClient(c client.Client) Client {
	return NewDefaultClient(c)
}

type Client interface {
	Cluster(clusterName string) Client
	PullSecret() (*corev1.Secret, error)
	ImageOverride(imageName string) (string, error)
}

type DefaultClient struct {
	client  client.Client
	cluster string
}

func NewDefaultClient(client client.Client) Client {
	return &DefaultClient{
		client: client,
	}
}

func (c *DefaultClient) Cluster(clusterName string) Client {
	return &DefaultClient{
		client:  c.client,
		cluster: clusterName,
	}
}

// PullSecret returns the pullSecret.
// return nil if there is no imageRegistry of the cluster.
func (c *DefaultClient) PullSecret() (*corev1.Secret, error) {
	imageRegistries, err := c.getImageRegistries(c.cluster)
	if err != nil {
		return nil, err
	}
	segs := strings.Split(imageRegistries.PullSecret, ".")
	if len(segs) != 2 {
		return nil, fmt.Errorf("wrong pullSecret format %v in the annotation %s",
			imageRegistries.PullSecret, ClusterImageRegistriesAnnotation)
	}
	namespace := segs[0]
	pullSecretName := segs[1]

	pullSecret := &corev1.Secret{}
	err = c.client.Get(context.TODO(), types.NamespacedName{Name: pullSecretName, Namespace: namespace}, pullSecret)
	if err != nil {
		return nil, err
	}

	return pullSecret, nil
}

// ImageOverride returns the overridden image.
// return the input image name if there is no custom registry.
func (c *DefaultClient) ImageOverride(imageName string) (newImageName string, err error) {
	imageRegistries, err := c.getImageRegistries(c.cluster)
	if err != nil {
		return imageName, err
	}

	if len(imageRegistries.Registries) == 0 {
		return imageName, nil
	}
	overrideImageName := imageName
	for i := 0; i < len(imageRegistries.Registries); i++ {
		registry := imageRegistries.Registries[i]
		name := imageOverride(registry.Source, registry.Mirror, imageName)
		if name != imageName {
			overrideImageName = name
		}
	}
	return overrideImageName, nil
}

// getImageRegistries retrieves the imageRegistries from annotations of managedCluster.
func (c *DefaultClient) getImageRegistries(clusterName string) (ImageRegistries, error) {
	imageRegistries := ImageRegistries{}
	managedCluster := &clusterv1.ManagedCluster{}
	err := c.client.Get(context.TODO(), types.NamespacedName{Name: clusterName}, managedCluster)
	if err != nil {
		return imageRegistries, err
	}
	annotations := managedCluster.GetAnnotations()
	if len(annotations) == 0 {
		return imageRegistries, nil
	}

	if annotations[ClusterImageRegistriesAnnotation] == "" {
		return imageRegistries, nil
	}

	err = json.Unmarshal([]byte(annotations[ClusterImageRegistriesAnnotation]), &imageRegistries)
	return imageRegistries, err
}

func imageOverride(source, mirror, imageName string) string {
	source = strings.TrimSuffix(source, "/")
	mirror = strings.TrimSuffix(mirror, "/")
	imageSegments := strings.Split(imageName, "/")
	imageNameTag := imageSegments[len(imageSegments)-1]
	if source == "" {
		if mirror == "" {
			return imageNameTag
		}
		return fmt.Sprintf("%s/%s", mirror, imageNameTag)
	}

	if !strings.HasPrefix(imageName, source) {
		return imageName
	}

	trimSegment := strings.TrimPrefix(imageName, source)
	return fmt.Sprintf("%s%s", mirror, trimSegment)
}
