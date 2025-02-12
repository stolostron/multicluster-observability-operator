// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"bytes"
	"context"
	"io"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func GetPodList(opt TestOptions, isHub bool, namespace string, labelSelector string) (error, *v1.PodList) {
	clientKube := getKubeClient(opt, isHub)
	listOption := metav1.ListOptions{}
	cluster := opt.HubCluster.BaseDomain
	if !isHub {
		cluster = opt.ManagedClusters[0].BaseDomain
	}

	klog.Info("Get pod list in namespace ", namespace, " using labelselector ", labelSelector, " on cluster: ", cluster)
	if labelSelector != "" {
		listOption.LabelSelector = labelSelector
	}
	podList, err := clientKube.CoreV1().Pods(namespace).List(context.TODO(), listOption)
	if err != nil {
		klog.Errorf(
			"Failed to get pod list in namespace %s using labelselector %s due to %v",
			namespace,
			labelSelector,
			err,
		)
		return err, podList
	}
	if podList != nil && len(podList.Items) == 0 {
		klog.V(1).Infof("No pod found for labelselector %s namespace %s", labelSelector, namespace)
	}
	return nil, podList
}

func GetPodLogs(
	opt TestOptions,
	isHub bool,
	namespace, podName, containerName string,
	previous bool,
	tailLines int64,
) (string, error) {
	clientKube := getKubeClient(opt, isHub)
	podLogOpts := v1.PodLogOptions{
		Container: containerName,
		Previous:  previous,
		TailLines: &tailLines,
	}
	req := clientKube.CoreV1().Pods(namespace).GetLogs(podName, &podLogOpts)
	podLogs, err := req.Stream(context.TODO())
	if err != nil {
		klog.Errorf("Failed to get logs for %s/%s in namespace %s due to %v", podName, containerName, namespace, err)
		return "", err
	}

	defer func() {
		err := podLogs.Close()
		if err != nil {
			klog.Errorf("Failed to close pod logs due to %v", err)
		}
	}()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		klog.Errorf("Failed to copy pod logs to buffer due to %v", err)
		return "", err
	}
	return buf.String(), nil
}
