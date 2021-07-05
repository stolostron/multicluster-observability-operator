// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func CreateConfigMap(opt TestOptions, isHub bool, cm *corev1.ConfigMap) error {
	clientKube := getKubeClient(opt, isHub)
	found, err := clientKube.CoreV1().ConfigMaps(cm.ObjectMeta.Namespace).Get(cm.ObjectMeta.Name, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) {
		_, err := clientKube.CoreV1().ConfigMaps(cm.ObjectMeta.Namespace).Create(cm)
		if err == nil {
			klog.V(1).Infof("configmap %s created", cm.ObjectMeta.Name)
		}
		return err
	}
	if err != nil {
		return err
	}
	cm.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
	_, err = clientKube.CoreV1().ConfigMaps(cm.ObjectMeta.Namespace).Update(cm)
	if err == nil {
		klog.V(1).Infof("configmap %s updated", cm.ObjectMeta.Name)
	}
	return err
}

func GetConfigMap(opt TestOptions, isHub bool, name string,
	namespace string) (error, *corev1.ConfigMap) {
	clientKube := getKubeClient(opt, isHub)
	cm, err := clientKube.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get configmap %s in namespace %s due to %v", name, namespace, err)
	}
	return err, cm
}

func DeleteConfigMap(opt TestOptions, isHub bool, name string, namespace string) error {
	clientKube := getKubeClient(opt, isHub)
	err := clientKube.CoreV1().ConfigMaps(namespace).Delete(name, &metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Failed to delete configmap %s in namespace %s due to %v", name, namespace, err)
	}
	return err
}
