// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// CreateKubeClient creates kube client
func CreateKubeClient() (kubernetes.Interface, error) {
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.Error(err, "Failed to create the config")
		return nil, err
	}

	// generate the client based off of the config
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error(err, "Failed to create kube client")
		return nil, err
	}

	return kubeClient, err
}

// CreateOCPClient creates kocp client
func CreateOCPClient() (ocpClientSet.Interface, error) {
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.Error(err, "Failed to create the config")
		return nil, err
	}

	// generate the client based off of the config
	ocpClient, err := ocpClientSet.NewForConfig(config)
	if err != nil {
		log.Error(err, "Failed to create ocp config client")
		return nil, err
	}

	return ocpClient, err
}

// NewFakeClient creates new fake client for test purpose
func NewFakeClient(gvs []schema.GroupVersion, types []runtime.Object) client.Client {
	s := scheme.Scheme
	for k, gv := range gvs {
		s.AddKnownTypes(gv, types[k])
	}
	return fake.NewFakeClientWithScheme(s, types...)
}
