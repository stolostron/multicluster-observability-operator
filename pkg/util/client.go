// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	crdClientSet "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	crdClient crdClientSet.Interface
)

// CreateOCPClient creates ocp client
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

// createCRDClient creates CRD client
func getOrCreateCRDClient() (crdClientSet.Interface, error) {
	if crdClient != nil {
		return crdClient, nil
	}
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.Error(err, "Failed to create the config")
		return nil, err
	}

	// generate the client based off of the config
	crdClient, err = crdClientSet.NewForConfig(config)
	if err != nil {
		log.Error(err, "Failed to create CRD config client")
		return nil, err
	}

	return crdClient, err
}

func CheckCRDExist(crdName string) (bool, error) {
	crdClient, err := getOrCreateCRDClient()
	if err != nil {
		log.Error(err, "Failed to get or create CRD config client")
		return false, err
	}

	_, err = crdClient.ApiextensionsV1beta1().CustomResourceDefinitions().Get(context.TODO(), crdName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("unable to get CRD with ApiextensionsV1beta1 Client, not found, will try to get it with ApiextensionsV1 Client.")
			_, err = crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), crdName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					log.Info("unable to get CRD with ApiextensionsV1 Client, not found.")
					return false, nil
				}
				log.Error(err, "failed to get PlacementRule CRD with ApiextensionsV1 Client")
				// ignore the error since only care if the CRD exists or not
				return false, nil
			}
			return true, nil
		} else {
			log.Error(err, "failed to get PlacementRule CRD with ApiextensionsV1beta1 Client")
			// ignore the error since only care if the CRD exists or not
			return false, nil
		}
	}
	return true, nil
}
