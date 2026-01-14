// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func AddToScheme(s *runtime.Scheme) error {
	if err := clusterv1.Install(s); err != nil {
		return err
	}

	return nil
}

func newFakeCluster(name, imageRegistryAnnotation string) *clusterv1.ManagedCluster {
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if imageRegistryAnnotation != "" {
		cluster.SetAnnotations(map[string]string{ClusterImageRegistriesAnnotation: imageRegistryAnnotation})
	}
	return cluster
}

func newAnnotationRegistries(registries []Registry, namespacePullSecret string) string {
	registriesData := ImageRegistries{
		PullSecret: namespacePullSecret,
		Registries: registries,
	}

	registriesDataStr, _ := json.Marshal(registriesData)
	return string(registriesDataStr)
}

func Test_DefaultClientPullSecret(t *testing.T) {
	testCases := []struct {
		name               string
		pullSecret         *corev1.Secret
		clusterName        string
		cluster            *clusterv1.ManagedCluster
		expectedErr        error
		expectedPullSecret *corev1.Secret
	}{
		{
			name:               "get correct pullSecret",
			pullSecret:         newPullSecret("pullSecret", "ns1", []byte("data")),
			clusterName:        "cluster1",
			cluster:            newFakeCluster("cluster1", newAnnotationRegistries(nil, "ns1.pullSecret")),
			expectedErr:        nil,
			expectedPullSecret: newPullSecret("pullSecret", "ns1", []byte("data")),
		},
		{
			name:               "failed to get pullSecret without annotation",
			pullSecret:         newPullSecret("pullSecret", "ns1", []byte("data")),
			clusterName:        "cluster1",
			cluster:            newFakeCluster("cluster1", ""),
			expectedErr:        fmt.Errorf("wrong pullSecret format  in the annotation %s", ClusterImageRegistriesAnnotation),
			expectedPullSecret: nil,
		},
		{
			name:               "failed to get pullSecret with wrong annotation",
			pullSecret:         newPullSecret("pullSecret", "ns1", []byte("data")),
			clusterName:        "cluster1",
			cluster:            newFakeCluster("cluster1", "abc"),
			expectedErr:        errors.New("invalid character 'a' looking for beginning of value"),
			expectedPullSecret: nil,
		},
		{
			name:               "failed to get pullSecret with wrong cluster",
			pullSecret:         newPullSecret("pullSecret", "ns1", []byte("data")),
			clusterName:        "cluster1",
			cluster:            newFakeCluster("cluster2", ""),
			expectedErr:        errors.New(`managedclusters.cluster.open-cluster-management.io "cluster1" not found`),
			expectedPullSecret: nil,
		},
		{
			name:               "failed to get pullSecret without pullSecret",
			pullSecret:         newPullSecret("pullSecret", "ns1", []byte("data")),
			clusterName:        "cluster1",
			cluster:            newFakeCluster("cluster1", newAnnotationRegistries(nil, "ns.test")),
			expectedErr:        errors.New("secrets \"test\" not found"),
			expectedPullSecret: nil,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{})
			_ = AddToScheme(scheme)
			client := NewDefaultClient(fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(c.cluster, c.pullSecret).Build())
			pullSecret, err := client.Cluster(c.clusterName).PullSecret()
			if err != nil && c.expectedErr != nil {
				if err.Error() != c.expectedErr.Error() {
					t.Errorf("expected err %v, but got %v", c.expectedErr, err)
				}
			}

			if pullSecret == nil && c.expectedPullSecret != nil {
				t.Errorf("expected pullSecretData %+v,but got %+v", c.expectedPullSecret, pullSecret)
			}
			if pullSecret != nil {
				pullSecret.SetResourceVersion("")
				if !equality.Semantic.DeepEqual(c.expectedPullSecret, pullSecret) {
					t.Errorf("expected pullSecretData %#v,but got %#v", c.expectedPullSecret, pullSecret)
				}

			}
		})
	}
}

func Test_DefaultClientImageOverride(t *testing.T) {
	testCases := []struct {
		name          string
		image         string
		clusterName   string
		cluster       *clusterv1.ManagedCluster
		expectedImage string
		expectedErr   error
	}{
		{
			name:        "override rhacm2 image ",
			clusterName: "cluster1",
			cluster: newFakeCluster("cluster1", newAnnotationRegistries([]Registry{
				{Source: "registry.redhat.io/rhacm2", Mirror: "quay.io/rhacm2"},
				{Source: "registry.redhat.io/multicluster-engine", Mirror: "quay.io/multicluster-engine"},
			}, "")),
			image:         "registry.redhat.io/rhacm2/registration@SHA256abc",
			expectedImage: "quay.io/rhacm2/registration@SHA256abc",
			expectedErr:   nil,
		},
		{
			name:        "override acm-d image",
			clusterName: "cluster1",
			cluster: newFakeCluster("cluster1", newAnnotationRegistries([]Registry{
				{Source: "registry.redhat.io/rhacm2", Mirror: "quay.io/rhacm2"},
				{Source: "registry.redhat.io/multicluster-engine", Mirror: "quay.io/multicluster-engine"},
			}, "")),
			image:         "registry.redhat.io/acm-d/registration@SHA256abc",
			expectedImage: "registry.redhat.io/acm-d/registration@SHA256abc",
			expectedErr:   nil,
		},
		{
			name:        "override multicluster-engine image",
			clusterName: "cluster1",
			cluster: newFakeCluster("cluster1", newAnnotationRegistries([]Registry{
				{Source: "registry.redhat.io/rhacm2", Mirror: "quay.io/rhacm2"},
				{Source: "registry.redhat.io/multicluster-engine", Mirror: "quay.io/multicluster-engine"},
			}, "")),
			image:         "registry.redhat.io/multicluster-engine/registration@SHA256abc",
			expectedImage: "quay.io/multicluster-engine/registration@SHA256abc",
			expectedErr:   nil,
		},
		{
			name:        "override image without source ",
			clusterName: "cluster1",
			cluster: newFakeCluster("cluster1", newAnnotationRegistries([]Registry{
				{Source: "", Mirror: "quay.io/rhacm2"},
			}, "")),
			image:         "registry.redhat.io/multicluster-engine/registration@SHA256abc",
			expectedImage: "quay.io/rhacm2/registration@SHA256abc",
			expectedErr:   nil,
		},
		{
			name:        "override image",
			clusterName: "cluster1",
			cluster: newFakeCluster("cluster1", newAnnotationRegistries([]Registry{
				{Source: "registry.redhat.io/rhacm2", Mirror: "quay.io/rhacm2"},
				{
					Source: "registry.redhat.io/rhacm2/registration@SHA256abc",
					Mirror: "quay.io/acm-d/registration:latest",
				},
			}, "")),
			image:         "registry.redhat.io/rhacm2/registration@SHA256abc",
			expectedImage: "quay.io/acm-d/registration:latest",
			expectedErr:   nil,
		},
		{
			name:          "return image without annotation",
			clusterName:   "cluster1",
			cluster:       newFakeCluster("cluster1", ""),
			image:         "registry.redhat.io/rhacm2/registration@SHA256abc",
			expectedImage: "registry.redhat.io/rhacm2/registration@SHA256abc",
			expectedErr:   nil,
		},
		{
			name:          "return image with wrong annotation",
			clusterName:   "cluster1",
			cluster:       newFakeCluster("cluster1", "abc"),
			image:         "registry.redhat.io/rhacm2/registration@SHA256abc",
			expectedImage: "registry.redhat.io/rhacm2/registration@SHA256abc",
			expectedErr:   errors.New("invalid character 'a' looking for beginning of value"),
		},
		{
			name:          "return image without cluster",
			clusterName:   "cluster1",
			cluster:       newFakeCluster("cluster2", ""),
			image:         "registry.redhat.io/rhacm2/registration@SHA256abc",
			expectedImage: "registry.redhat.io/rhacm2/registration@SHA256abc",
			expectedErr:   errors.New(`managedclusters.cluster.open-cluster-management.io "cluster1" not found`),
		},
	}

	pullSecret := newPullSecret("pullSecret", "ns1", []byte("data"))
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{})
			_ = AddToScheme(scheme)

			client := NewDefaultClient(fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(c.cluster, pullSecret).Build())
			newImage, err := client.Cluster(c.clusterName).ImageOverride(c.image)
			if err != nil && c.expectedErr != nil {
				if err.Error() != c.expectedErr.Error() {
					t.Errorf("expected err %v, but got %v", c.expectedErr, err)
				}
			}
			if newImage != c.expectedImage {
				t.Errorf("execpted image %v, but got %v", c.expectedImage, newImage)
			}
		})
	}
}
