// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func GetCRB(opt TestOptions, isHub bool, name string) (error, *rbacv1.ClusterRoleBinding) {
	clientKube := getKubeClient(opt, isHub)
	crb, err := clientKube.RbacV1().ClusterRoleBindings().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get cluster rolebinding %s due to %v", name, err)
	}
	return err, crb
}

func DeleteCRB(opt TestOptions, isHub bool, name string) error {
	clientKube := getKubeClient(opt, isHub)
	err := clientKube.RbacV1().ClusterRoleBindings().Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Failed to delete cluster rolebinding %s due to %v", name, err)
	}
	return err
}

func UpdateCRB(opt TestOptions, isHub bool, name string,
	crb *rbacv1.ClusterRoleBinding) (error, *rbacv1.ClusterRoleBinding) {
	clientKube := getKubeClient(opt, isHub)
	updateCRB, err := clientKube.RbacV1().ClusterRoleBindings().Update(context.TODO(), crb, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Failed to update cluster rolebinding %s due to %v", name, err)
	}
	return err, updateCRB
}

func CreateCRB(opt TestOptions, isHub bool,
	crb *rbacv1.ClusterRoleBinding) error {
	clientKube := getKubeClient(opt, isHub)
	_, err := clientKube.RbacV1().ClusterRoleBindings().Create(context.TODO(), crb, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			klog.V(1).Infof("clusterrolebinding %s already exists, updating...", crb.GetName())
			err, _ := UpdateCRB(opt, isHub, crb.GetName(), crb)
			return err
		}
		klog.Errorf("Failed to create cluster rolebinding %s due to %v", crb.GetName(), err)
		return err
	}
	return nil
}
