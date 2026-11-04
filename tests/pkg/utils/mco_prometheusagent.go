// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"fmt"

	appv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewPrometheusAgentGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "monitoring.rhobs",
		Version:  "v1alpha1",
		Resource: "prometheusagents",
	}
}

// CheckNonOCPPrometheusAgentReady reports whether the non-OCP configuration has
// reached a ready replacement StatefulSet pod.
func CheckNonOCPPrometheusAgentReady(ctx context.Context, cluster Cluster, agentName, statefulSetName string) error {
	pa, err := GetKubeClientDynamicWithCluster(cluster).Resource(NewPrometheusAgentGVR()).
		Namespace(MCO_AGENT_ADDON_NAMESPACE).Get(ctx, agentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get PrometheusAgent %s/%s: %w", MCO_AGENT_ADDON_NAMESPACE, agentName, err)
	}

	sts, err := GetKubeClientWithCluster(cluster).AppsV1().StatefulSets(MCO_AGENT_ADDON_NAMESPACE).
		Get(ctx, statefulSetName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get StatefulSet %s/%s: %w", MCO_AGENT_ADDON_NAMESPACE, statefulSetName, err)
	}

	return checkNonOCPPrometheusAgentReady(pa, sts)
}

func checkNonOCPPrometheusAgentReady(pa *unstructured.Unstructured, sts *appv1.StatefulSet) error {
	if pa == nil {
		return fmt.Errorf("PrometheusAgent is nil")
	}
	if sts == nil {
		return fmt.Errorf("StatefulSet is nil")
	}

	scrapeClasses, _, err := unstructured.NestedSlice(pa.Object, "spec", "scrapeClasses")
	if err != nil {
		return fmt.Errorf("read PrometheusAgent %s scrape classes: %w", pa.GetName(), err)
	}
	nonOCPDefault := false
	for _, item := range scrapeClasses {
		scrapeClass, ok := item.(map[string]any)
		if ok && scrapeClass["name"] == "non-ocp-monitoring" && scrapeClass["default"] == true {
			nonOCPDefault = true
			break
		}
	}
	if !nonOCPDefault {
		return fmt.Errorf("PrometheusAgent %s does not have the default non-ocp-monitoring scrape class", pa.GetName())
	}

	conditions, _, err := unstructured.NestedSlice(pa.Object, "status", "conditions")
	if err != nil {
		return fmt.Errorf("read PrometheusAgent %s conditions: %w", pa.GetName(), err)
	}
	available := false
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if ok && condition["type"] == "Available" && condition["status"] == "True" {
			available = true
			break
		}
	}
	if !available {
		return fmt.Errorf("PrometheusAgent %s is not Available", pa.GetName())
	}

	for _, container := range sts.Spec.Template.Spec.Containers {
		if container.Name == "kube-rbac-proxy" {
			return fmt.Errorf("StatefulSet %s still has the OpenShift-only kube-rbac-proxy sidecar", sts.GetName())
		}
	}

	expectedReplicas := int32(1)
	if sts.Spec.Replicas != nil {
		expectedReplicas = *sts.Spec.Replicas
	}
	if expectedReplicas == 0 || sts.Status.ObservedGeneration < sts.Generation ||
		sts.Status.UpdatedReplicas != expectedReplicas || sts.Status.ReadyReplicas != expectedReplicas ||
		sts.Status.CurrentRevision != sts.Status.UpdateRevision {
		return fmt.Errorf("StatefulSet %s non-OCP rollout is not ready: observed generation %d/%d, updated %d/%d, ready %d/%d, revisions %q/%q",
			sts.GetName(), sts.Status.ObservedGeneration, sts.Generation, sts.Status.UpdatedReplicas, expectedReplicas,
			sts.Status.ReadyReplicas, expectedReplicas, sts.Status.CurrentRevision, sts.Status.UpdateRevision)
	}

	return nil
}

func UpdatePrometheusAgentScrapeInterval(opt TestOptions, labelSelector string, interval string) error {
	clientDynamic := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	listOpt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/component=%s", labelSelector),
	}
	paList, err := clientDynamic.Resource(NewPrometheusAgentGVR()).Namespace(MCO_NAMESPACE).List(context.TODO(), listOpt)
	if err != nil {
		return err
	}

	if len(paList.Items) != 1 {
		return fmt.Errorf("expected 1 PrometheusAgent with label app.kubernetes.io/component=%s, but found %d", labelSelector, len(paList.Items))
	}

	pa := paList.Items[0]
	if err := unstructured.SetNestedField(pa.Object, interval, "spec", "scrapeInterval"); err != nil {
		return fmt.Errorf("failed to set scrapeInterval on PrometheusAgent: %w", err)
	}
	if err := unstructured.SetNestedField(pa.Object, interval, "spec", "scrapeTimeout"); err != nil {
		return fmt.Errorf("failed to set scrapeTimeout on PrometheusAgent: %w", err)
	}

	_, err = clientDynamic.Resource(NewPrometheusAgentGVR()).Namespace(MCO_NAMESPACE).Update(context.TODO(), &pa, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update PrometheusAgent %s: %w", pa.GetName(), err)
	}

	return nil
}
