// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ocpClientSet "github.com/openshift/client-go/config/clientset/versioned"
	crdClientSet "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	crdClient crdClientSet.Interface
	ocpClient ocpClientSet.Interface
)

// GetOrCreateOCPClient creates ocp client
func GetOrCreateOCPClient() (ocpClientSet.Interface, error) {
	if crdClient != nil {
		return ocpClient, nil
	}
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.Error(err, "Failed to create the config")
		return nil, err
	}

	// generate the client based off of the config
	ocpClient, err = ocpClientSet.NewForConfig(config)
	if err != nil {
		log.Error(err, "Failed to create ocp config client")
		return nil, err
	}

	return ocpClient, err
}

// GetOrCreateCRDClient gets an existing or creates a new CRD client
func GetOrCreateCRDClient() (crdClientSet.Interface, error) {
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

func CheckCRDExist(crdClient crdClientSet.Interface, crdName string) (bool, error) {
	log.Info("unable to get CRD with ApiextensionsV1beta1 Client, not found, will try to get it with ApiextensionsV1 Client.")
	_, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), crdName, metav1.GetOptions{})
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
}

func UpdateCRDWebhookNS(crdClient crdClientSet.Interface, namespace, crdName string) error {
	crdObj, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), crdName, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "failed to get CRD", "CRD", crdName)
		return err
	}
	if crdObj.Spec.Conversion == nil || crdObj.Spec.Conversion.Webhook == nil || crdObj.Spec.Conversion.Webhook.ClientConfig == nil {
		log.Error(err, "empty Conversion in the CRD", "CRD", crdName)
		return fmt.Errorf("empty Conversion in the CRD %s", crdName)
	}
	if crdObj.Spec.Conversion.Webhook.ClientConfig.Service.Namespace != namespace {
		log.Info("updating the webhook service namespace", "original namespace", crdObj.Spec.Conversion.Webhook.ClientConfig.Service.Namespace, "new namespace", namespace)
		crdObj.Spec.Conversion.Webhook.ClientConfig.Service.Namespace = namespace
		_, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Update(context.TODO(), crdObj, metav1.UpdateOptions{})
		if err != nil {
			log.Error(err, "failed to update webhook service namespace")
			return err
		}
	}
	return nil
}
