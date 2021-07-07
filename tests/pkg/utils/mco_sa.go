// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func DeleteSA(opt TestOptions, isHub bool, namespace string,
	name string) error {
	clientKube := getKubeClient(opt, isHub)
	err := clientKube.CoreV1().ServiceAccounts(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Failed to delete serviceaccount %s due to %v", name, err)
	}
	return err
}

func UpdateSA(opt TestOptions, isHub bool, namespace string,
	sa *v1.ServiceAccount) (error, *v1.ServiceAccount) {
	clientKube := getKubeClient(opt, isHub)
	updateSA, err := clientKube.CoreV1().ServiceAccounts(namespace).Update(context.TODO(), sa, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Failed to update serviceaccount %s due to %v", sa.GetName(), err)
	}
	return err, updateSA
}

func CreateSA(opt TestOptions, isHub bool, namespace string,
	sa *v1.ServiceAccount) error {
	clientKube := getKubeClient(opt, isHub)
	_, err := clientKube.CoreV1().ServiceAccounts(namespace).Create(context.TODO(), sa, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			klog.V(1).Infof("serviceaccount %s already exists, updating...", sa.GetName())
			err, _ := UpdateSA(opt, isHub, namespace, sa)
			return err
		}
		klog.Errorf("Failed to create serviceaccount %s due to %v", sa.GetName(), err)
		return err
	}
	return nil
}
