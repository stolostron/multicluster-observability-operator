// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"context"

	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("util")

// Remove is used to remove string from a string array
func Remove(list []string, s string) []string {
	result := []string{}
	for _, v := range list {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}

// Contains is used to check whether a list contains string s
func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// GetAnnotation returns the annotation value for a given key, or an empty string if not set
func GetAnnotation(annotations map[string]string, key string) string {
	if annotations == nil {
		return ""
	}
	return annotations[key]
}

// GetPVCList get pvc with matched labels
func GetPVCList(c client.Client, matchLabels map[string]string) ([]corev1.PersistentVolumeClaim, error) {
	pvcList := &corev1.PersistentVolumeClaimList{}
	pvcListOpts := []client.ListOption{
		client.InNamespace(config.GetDefaultNamespace()),
		client.MatchingLabels(matchLabels),
	}

	err := c.List(context.TODO(), pvcList, pvcListOpts...)
	if err != nil {
		return nil, err
	}
	return pvcList.Items, nil
}

// GetStatefulSetList get sts with matched labels
func GetStatefulSetList(c client.Client, matchLabels map[string]string) ([]appsv1.StatefulSet, error) {
	stsList := &appsv1.StatefulSetList{}
	stsListOpts := []client.ListOption{
		client.InNamespace(config.GetDefaultNamespace()),
		client.MatchingLabels(matchLabels),
	}

	err := c.List(context.TODO(), stsList, stsListOpts...)
	if err != nil {
		return nil, err
	}
	return stsList.Items, nil
}
