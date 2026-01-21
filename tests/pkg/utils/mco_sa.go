// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func DeleteSA(opt TestOptions, isHub bool, namespace string,
	name string,
) error {
	clientKube := GetKubeClient(opt, isHub)
	err := clientKube.CoreV1().ServiceAccounts(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Failed to delete serviceaccount %s due to %v", name, err)
	}
	return err
}

func UpdateSA(opt TestOptions, isHub bool, namespace string,
	sa *v1.ServiceAccount,
) (*v1.ServiceAccount, error) {
	clientKube := GetKubeClient(opt, isHub)
	updateSA, err := clientKube.CoreV1().ServiceAccounts(namespace).Update(context.TODO(), sa, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Failed to update serviceaccount %s due to %v", sa.GetName(), err)
	}
	return updateSA, err
}

func CreateSA(opt TestOptions, isHub bool, namespace string,
	sa *v1.ServiceAccount,
) error {
	clientKube := GetKubeClient(opt, isHub)
	_, err := clientKube.CoreV1().ServiceAccounts(namespace).Create(context.TODO(), sa, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			klog.V(1).Infof("serviceaccount %s already exists, updating...", sa.GetName())
			_, err := UpdateSA(opt, isHub, namespace, sa)
			return err
		}
		klog.Errorf("Failed to create serviceaccount %s due to %v", sa.GetName(), err)
		return err
	}
	return nil
}

func GetSAWithLabel(opt TestOptions, isHub bool, label string,
	namespace string,
) (*v1.ServiceAccountList, error) {
	clientKube := GetKubeClient(opt, isHub)
	klog.V(1).Infof("Get get sa with label selector <%v> in namespace <%v>, isHub: <%v>",
		label,
		namespace,
		isHub)
	sas, err := clientKube.CoreV1().ServiceAccounts(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		klog.Errorf("Failed to get ServiceAccount with label selector %s in namespace %s due to %v", label, namespace, err)
	}

	return sas, err
}
