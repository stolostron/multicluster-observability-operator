// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

var klogSeverityPattern = regexp.MustCompile(`^[EFW]\d{4} \d{2}:\d{2}:\d{2}`)

const (
	DebugDumpStartMarker          = "==================== [DEBUG DUMP START] ===================="
	DebugDumpEndMarker            = "==================== [DEBUG DUMP END] ======================"
	SectionMCOCR                  = "---------- [SECTION: MCO CR] ----------"
	SectionManagedClusters        = "---------- [SECTION: ManagedClusters] ----------"
	SectionManagedClusterAddOns   = "---------- [SECTION: ManagedClusterAddOns] ----------"
	SectionClusterManagementAddOn = "---------- [SECTION: ClusterManagementAddOn] ----------"
	SectionManifestWorks          = "---------- [SECTION: Observability ManifestWorks] ----------"
	SectionAddOnDeploymentConfigs = "---------- [SECTION: AddOnDeploymentConfigs] ----------"
	SectionHubWorkloads           = "---------- [SECTION: Hub Workloads & Pods] ----------"
	SectionSpokeWorkloads         = "---------- [SECTION: Spoke Workloads & Pods] ----------"
	statusUnknown                 = "Unknown"
	statusTrue                    = "True"
	statusFalse                   = "False"
	conditionAvailable            = "Available"
	conditionDegraded             = "Degraded"
	conditionApplied              = "Applied"
	conditionProgressing          = "Progressing"
)

// cleanUnstructuredForLogging strips managedFields and bulky annotations (e.g. kubectl last-applied)
// to prevent massive token bloat when logging Kubernetes objects.
func cleanUnstructuredForLogging(obj *unstructured.Unstructured) {
	if obj == nil || obj.Object == nil {
		return
	}
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	annotations := obj.GetAnnotations()
	if annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		obj.SetAnnotations(annotations)
	}
}

// logMCOStatus provides a concise summary of MCO CR capabilities and conditions.
func logMCOStatus(client dynamic.Interface) {
	gvr := NewMCOGVRV1BETA2()
	obj, err := client.Resource(gvr).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
	if err != nil {
		klog.V(1).Infof("Failed to get MultiClusterObservability %s: %v", MCO_CR_NAME, err)
		return
	}

	cleanUnstructuredForLogging(obj)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("MultiClusterObservability %q (generation %d):\n", MCO_CR_NAME, obj.GetGeneration()))

	capabilities, found, _ := unstructured.NestedMap(obj.Object, "spec", "capabilities")
	if found {
		capJSON, _ := json.Marshal(capabilities)
		sb.WriteString(fmt.Sprintf("  Capabilities: %s\n", string(capJSON)))
	}

	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if found {
		sb.WriteString("  Conditions:\n")
		for _, c := range conditions {
			if cMap, ok := c.(map[string]any); ok {
				sb.WriteString(fmt.Sprintf("    - %s: %s (%s: %s)\n",
					cMap["type"], cMap["status"], cMap["reason"], cMap["message"]))
			}
		}
	}
	klog.Info(sb.String())
}

// LogFailingTestStandardDebugInfo logs standard debug info for failing tests.
// It scans workloads and pods from hub and managed clusters observability namespaces.
// It also prints MCO, MCA, CMAO, and ManifestWork objects.
func LogFailingTestStandardDebugInfo(opt TestOptions, isMCOA bool) {
	klog.Info(DebugDumpStartMarker)
	klog.Infof("Failing Test Debug Info | Hub: %s | ManagedClusters: %d | MCOA: %t",
		opt.HubCluster.ClusterServerURL, len(opt.ManagedClusters), isMCOA)

	hubDynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	hubClient := NewKubeClient(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)

	// Section 1: MCO CR
	klog.Info(SectionMCOCR)
	logMCOStatus(hubDynClient)
	PrintObject(context.TODO(), hubDynClient, NewMCOGVRV1BETA2(), "", MCO_CR_NAME)

	// Section 2: Managed Clusters
	klog.Info(SectionManagedClusters)
	LogManagedClusters(hubDynClient)

	// Section 3: Addon Management & ManifestWorks (Hub-Side Spoke Diagnostics)
	klog.Info(SectionManagedClusterAddOns)
	LogManagedClusterAddOns(hubDynClient)

	if isMCOA {
		klog.Info(SectionClusterManagementAddOn)
		LogClusterManagementAddOn(hubDynClient)
	}

	klog.Info(SectionManifestWorks)
	printManifestWorks(hubDynClient)

	if isMCOA {
		klog.Info(SectionAddOnDeploymentConfigs)
		printAddonDeploymentConfigs(hubDynClient, MCO_NAMESPACE)
	}

	// Section 4: Hub Workloads & Pods
	klog.Info(SectionHubWorkloads)
	CheckPodsInNamespace(hubClient, "open-cluster-management", []string{"multicluster-observability-operator"}, map[string]string{
		"name": "multicluster-observability-operator",
	})
	CheckPodsInNamespace(hubClient, MCO_NAMESPACE, []string{"multicluster-observability-addon-manager"}, map[string]string{
		"app": "multicluster-observability-addon-manager",
	})
	CheckDeploymentsInNamespace(hubClient, MCO_NAMESPACE)
	CheckStatefulSetsInNamespace(hubClient, MCO_NAMESPACE)
	CheckDaemonSetsInNamespace(hubClient, MCO_NAMESPACE)
	CheckPodsInNamespace(hubClient, MCO_NAMESPACE, []string{}, map[string]string{})
	printConfigMapsInNamespace(hubClient, MCO_NAMESPACE)
	printSecretsInNamespace(hubClient, MCO_NAMESPACE)

	if isMCOA {
		CheckDeploymentsInNamespace(hubClient, MCO_AGENT_ADDON_NAMESPACE)
		CheckStatefulSetsInNamespace(hubClient, MCO_AGENT_ADDON_NAMESPACE)
		CheckPodsInNamespace(hubClient, MCO_AGENT_ADDON_NAMESPACE, []string{"endpoint-monitoring-operator", "observability-monitoring-cleanup"}, map[string]string{})
	}

	// Section 5: Spoke Clusters
	inspectedClusters := make(map[string]bool)
	inspectedClusters["local-cluster"] = true

	// 5A: Explicitly configured managed clusters in TestOptions
	for _, mc := range opt.ManagedClusters {
		if inspectedClusters[mc.Name] {
			continue
		}
		inspectedClusters[mc.Name] = true

		spokeDynClient := NewKubeClientDynamic(mc.ClusterServerURL, mc.KubeConfig, mc.KubeContext)
		spokeClient := NewKubeClient(mc.ClusterServerURL, mc.KubeConfig, mc.KubeContext)
		logSpokeClusterDebugInfo(spokeClient, spokeDynClient, mc.Name, isMCOA)
	}

	// 5B: Dynamic Spoke Kubeconfig Resolution (for regression environments where opt.ManagedClusters is empty)
	discoveredClusters, err := GetDiscoveredManagedClusterNames(context.TODO(), hubDynClient)
	if err == nil {
		for _, clusterName := range discoveredClusters {
			if inspectedClusters[clusterName] {
				continue
			}
			inspectedClusters[clusterName] = true

			spokeClient, spokeDynClient, err := resolveSpokeClientsFromHub(context.TODO(), hubClient, hubDynClient, clusterName)
			if err != nil {
				klog.V(2).Infof("Could not dynamically resolve credentials for spoke cluster %s: %v", clusterName, err)
				continue
			}

			klog.Infof("Dynamically resolved spoke credentials for cluster %s from Hub", clusterName)
			logSpokeClusterDebugInfo(spokeClient, spokeDynClient, clusterName, isMCOA)
		}
	} else {
		klog.V(2).Infof("Failed to discover managed clusters on Hub: %v", err)
	}

	klog.Info(DebugDumpEndMarker)
}

// CheckPodsInNamespace lists pods in a namespace and logs debug info (status, events, logs) for pods not running.
func CheckPodsInNamespace(client kubernetes.Interface, ns string, forcePodNamesLog []string, podLabels map[string]string) {
	listOptions := metav1.ListOptions{}
	if len(podLabels) > 0 {
		listOptions.LabelSelector = metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: podLabels})
	}
	pods, err := client.CoreV1().Pods(ns).List(context.TODO(), listOptions)
	if err != nil {
		klog.Errorf("Failed to get pods in namespace %s: %v", ns, err)
		return
	}

	if len(pods.Items) == 0 {
		klog.V(1).Infof("No pods found in namespace %s", ns)
		return
	}

	klog.V(1).Infof("Checking %d pods in namespace %q", len(pods.Items), ns)
	printPodsStatuses(pods.Items)

	notRunningPodsCount := 0
	forcedPodsLogged := make(map[string]bool)

	for _, pod := range pods.Items {
		isRunningOrSucceeded := pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodSucceeded
		if !isRunningOrSucceeded {
			notRunningPodsCount++
		}

		force := false
		for _, forcePodName := range forcePodNamesLog {
			if strings.Contains(pod.Name, forcePodName) {
				if isRunningOrSucceeded {
					if !forcedPodsLogged[forcePodName] {
						force = true
						forcedPodsLogged[forcePodName] = true
					}
				} else {
					force = true
				}
				break
			}
		}

		// Skip healthy pods unless explicitly forced for logging
		if isRunningOrSucceeded && !force {
			continue
		}

		if !isRunningOrSucceeded {
			// Failing pod: dump status, events, and logs
			LogPodStatus(pod)
			LogObjectEvents(client, ns, "Pod", pod.Name)
			LogPodLogs(client, ns, pod)
		} else if force {
			// Running pod forced for logging: do NOT dump static spec or events, only check recent error logs or tail
			LogPodLogs(client, ns, pod)
		}
	}

	if notRunningPodsCount == 0 {
		klog.V(1).Infof("All pods are running in namespace %q", ns)
	} else {
		klog.Errorf("Found %d pods not running in namespace %q", notRunningPodsCount, ns)
	}
}

func LogPodStatus(podList corev1.Pod) {
	var podStatus strings.Builder
	podStatus.WriteString(">>>>>>>>>> pod status >>>>>>>>>>\n")
	podStatus.WriteString("Conditions:\n")
	for _, condition := range podList.Status.Conditions {
		podStatus.WriteString(fmt.Sprintf("\t%s: %s %v\n", condition.Type, condition.Status, condition.LastTransitionTime.Time))
	}
	podStatus.WriteString("ContainerStatuses:\n")
	for _, containerStatus := range podList.Status.ContainerStatuses {
		podStatus.WriteString(fmt.Sprintf("\t%s: %t %d %v\n", containerStatus.Name, containerStatus.Ready, containerStatus.RestartCount, containerStatus.State))
		if containerStatus.LastTerminationState.Terminated != nil {
			podStatus.WriteString(fmt.Sprintf("\t\tlastTerminated: %v\n", containerStatus.LastTerminationState.Terminated))
		}
	}
	podStatus.WriteString("<<<<<<<<<< pod status <<<<<<<<<<")

	klog.V(1).Infof("Pod %q is in phase %q and status: \n%s", podList.Name, podList.Status.Phase, podStatus.String())
}

// isErrorLine identifies error, fatal, panic, failure, or timeout signatures.
func isErrorLine(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") ||
		strings.Contains(lower, "fatal") ||
		strings.Contains(lower, "panic") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "exception") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "timed out") {
		return true
	}
	return klogSeverityPattern.MatchString(line)
}

func LogPodLogs(client kubernetes.Interface, ns string, pod corev1.Pod) {
	for _, container := range pod.Spec.Containers {
		sinceSeconds := int64(360)
		limitBytes := int64(5 * 1024 * 1024)
		logsRes := client.CoreV1().Pods(ns).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container:    container.Name,
			Timestamps:   true,
			SinceSeconds: &sinceSeconds,
			LimitBytes:   &limitBytes,
		}).Do(context.Background())

		if logsRes.Error() != nil {
			klog.Errorf("Failed to get logs for pod %q container %q: %s", pod.Name, container.Name, logsRes.Error())
			continue
		}

		logs, err := logsRes.Raw()
		if err != nil {
			klog.Errorf("Failed to get logs for pod %q container %q: %s", pod.Name, container.Name, err.Error())
			continue
		}

		// Aggregate error/warning logs from the past 6 minutes
		var errorLines []string
		var windowLines []string
		lines := strings.Split(string(logs), "\n")
		cutoffTime := time.Now().Add(-6 * time.Minute)
		unparseableCount := 0

		for i := len(lines) - 1; i >= 0; i-- {
			line := lines[i]
			if line == "" {
				continue
			}

			// Try to parse timestamp at the beginning of the line
			timestampParsed := false
			fields := strings.Fields(line)
			if len(fields) > 0 {
				if t, err := time.Parse(time.RFC3339, fields[0]); err == nil {
					timestampParsed = true
					if t.Before(cutoffTime) {
						break
					}
				}
			}

			if !timestampParsed {
				if unparseableCount >= 100 {
					continue
				}
				unparseableCount++
			}

			windowLines = append(windowLines, line)
			if isErrorLine(line) {
				errorLines = append(errorLines, line)
			}
		}

		var displayLines []string
		msg := "recent errors and warnings from the past 6 minutes"
		if len(errorLines) > 0 {
			count := min(50, len(errorLines))
			displayLines = errorLines[:count]
			msg = fmt.Sprintf("%d error/warning lines from the past 6 minutes", count)
		} else if len(windowLines) > 0 {
			count := min(20, len(windowLines))
			displayLines = windowLines[:count]
			msg = fmt.Sprintf("last %d log lines (no errors detected in 6m window)", count)
		}

		// Reverse the lines to restore chronological order
		slices.Reverse(displayLines)

		if len(displayLines) > 0 {
			delimitedLogs := fmt.Sprintf(">>>>>>>>>> container logs: %s/%s >>>>>>>>>>\n%s\n<<<<<<<<<< container logs: %s/%s <<<<<<<<<<",
				pod.Name, container.Name, strings.Join(displayLines, "\n"), pod.Name, container.Name)
			klog.V(1).Infof("Pod %q container %q logs (%s): \n%s", pod.Name, container.Name, msg, delimitedLogs)
		}
	}
}

func CheckDeploymentsInNamespace(client kubernetes.Interface, ns string) {
	deployments, err := client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to get deployments in namespace %s: %v", ns, err)
		return
	}

	if len(deployments.Items) == 0 {
		klog.V(1).Infof("No deployments found in namespace %q", ns)
		return
	}

	klog.V(1).Infof("Deployments in namespace %s:\n", ns)
	printDeploymentsStatuses(client, ns)

	for _, deployment := range deployments.Items {
		desired := int32(1)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}

		if deployment.Status.ReadyReplicas == desired &&
			deployment.Status.AvailableReplicas == desired &&
			deployment.Status.UpdatedReplicas == desired {
			continue
		}

		LogDeploymentStatus(deployment)
		LogObjectEvents(client, ns, "Deployment", deployment.Name)
	}
}

func LogDeploymentStatus(deployment appsv1.Deployment) {
	var deploymentStatus strings.Builder
	deploymentStatus.WriteString(">>>>>>>>>> deployment status >>>>>>>>>>\n")
	deploymentStatus.WriteString(fmt.Sprintf("ReadyReplicas: %d\n", deployment.Status.ReadyReplicas))
	deploymentStatus.WriteString(fmt.Sprintf("UpdatedReplicas: %d\n", deployment.Status.UpdatedReplicas))
	deploymentStatus.WriteString(fmt.Sprintf("AvailableReplicas: %d\n", deployment.Status.AvailableReplicas))
	deploymentStatus.WriteString("Conditions:\n")
	for _, condition := range deployment.Status.Conditions {
		deploymentStatus.WriteString(fmt.Sprintf("\t%s: %s %v \n\t\t%s %s\n", condition.Type, condition.Status, condition.LastTransitionTime, condition.Message, condition.Reason))
	}
	deploymentStatus.WriteString("<<<<<<<<<< deployment status <<<<<<<<<<")

	klog.V(1).Infof("Deployment %q status: \n%s", deployment.Name, deploymentStatus.String())
}

func CheckStatefulSetsInNamespace(client kubernetes.Interface, ns string) {
	statefulSets, err := client.AppsV1().StatefulSets(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to get statefulsets in namespace %s: %v", ns, err)
		return
	}

	if len(statefulSets.Items) == 0 {
		klog.V(1).Infof("No statefulsets found in namespace %q", ns)
		return
	}

	klog.V(1).Infof("StatefulSets in namespace %s:\n", ns)
	printStatefulSetsStatuses(client, ns)

	for _, statefulSet := range statefulSets.Items {
		desired := int32(1)
		if statefulSet.Spec.Replicas != nil {
			desired = *statefulSet.Spec.Replicas
		}

		if statefulSet.Status.ReadyReplicas == desired &&
			statefulSet.Status.UpdatedReplicas == desired {
			continue
		}

		LogObjectEvents(client, ns, "StatefulSet", statefulSet.Name)
	}
}

func CheckDaemonSetsInNamespace(client kubernetes.Interface, ns string) {
	daemonSets, err := client.AppsV1().DaemonSets(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to get daemonsets in namespace %s: %v", ns, err)
		return
	}

	if len(daemonSets.Items) == 0 {
		klog.V(1).Infof("No daemonsets found in namespace %q", ns)
		return
	}

	klog.V(1).Infof("DaemonSets in namespace %s:\n", ns)
	printDaemonSetsStatuses(client, ns)

	for _, daemonSet := range daemonSets.Items {
		if daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled &&
			daemonSet.Status.UpdatedNumberScheduled == daemonSet.Status.DesiredNumberScheduled {
			continue
		}

		LogObjectEvents(client, ns, "DaemonSet", daemonSet.Name)
	}
}

func LogObjectEvents(client kubernetes.Interface, ns string, kind string, name string) {
	fieldSelector := fmt.Sprintf("involvedObject.kind=%s,involvedObject.name=%s", kind, name)
	events, err := client.CoreV1().Events(ns).List(context.TODO(), metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		klog.Errorf("Failed to get events for %s %s: %s", kind, name, err.Error())
		return
	}

	objectEvents := make([]string, 0, len(events.Items))
	for _, event := range events.Items {
		objectEvents = append(objectEvents, fmt.Sprintf("%s %s (%d): %s", event.Reason, event.LastTimestamp, event.Count, event.Message))
	}
	formattedEvents := fmt.Sprintf(">>>>>>>>>> %s events >>>>>>>>>>\n%s\n<<<<<<<<<< %s events <<<<<<<<<<", kind, strings.Join(objectEvents, "\n"), kind)
	klog.V(1).Infof("%s %q events: \n%s", kind, name, formattedEvents)
}

func LogManagedClusters(client dynamic.Interface) {
	objs, err := client.Resource(NewOCMManagedClustersGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to list ManagedClusters: %v", err)
		return
	}

	var sb strings.Builder
	sb.WriteString("Managed Clusters:\n")
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tAVAILABLE\tJOINED\tHUB-ACCEPTED\tVERSION\tURL")

	for _, obj := range objs.Items {
		managedCluster := &clusterv1.ManagedCluster{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, managedCluster); err != nil {
			klog.Errorf("Failed to convert unstructured to ManagedCluster %s: %v", obj.GetName(), err)
			continue
		}

		avail := statusUnknown
		joined := statusUnknown
		hubAccepted := statusUnknown
		for _, cond := range managedCluster.Status.Conditions {
			switch cond.Type {
			case clusterv1.ManagedClusterConditionAvailable:
				avail = string(cond.Status)
			case clusterv1.ManagedClusterConditionJoined:
				joined = string(cond.Status)
			case clusterv1.ManagedClusterConditionHubAccepted:
				hubAccepted = string(cond.Status)
			}
		}

		version := statusUnknown
		for _, claim := range managedCluster.Status.ClusterClaims {
			if claim.Name == "version.openshift.io" {
				version = claim.Value
				break
			}
		}
		if version == statusUnknown {
			for _, claim := range managedCluster.Status.ClusterClaims {
				if claim.Name == "platform.open-cluster-management.io" {
					version = claim.Value
					break
				}
			}
		}

		serverURL := ""
		if len(managedCluster.Spec.ManagedClusterClientConfigs) > 0 {
			serverURL = managedCluster.Spec.ManagedClusterClientConfigs[0].URL
		}

		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
			managedCluster.Name,
			avail,
			joined,
			hubAccepted,
			version,
			serverURL,
		)
	}
	_ = writer.Flush()
	klog.Info(sb.String())
}

// LogManagedClusterAddOns lists and displays status of ManagedClusterAddOn resources across all clusters.
func LogManagedClusterAddOns(client dynamic.Interface) {
	gvr := NewMCOManagedClusterAddonsGVR()
	objs, err := client.Resource(gvr).Namespace("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to list ManagedClusterAddOns: %v", err)
		return
	}

	var sb strings.Builder
	sb.WriteString("ManagedClusterAddOns:\n")
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "CLUSTER\tADDON\tAVAILABLE\tDEGRADED\tPROGRESSING\tDELETING\tFINALIZERS")

	type degradedAddon struct {
		cluster string
		addon   string
		detail  string
	}
	var degraded []degradedAddon

	for _, obj := range objs.Items {
		name := obj.GetName()
		if !strings.Contains(name, "observability") {
			continue
		}
		cluster := obj.GetNamespace()
		deleting := "No"
		if obj.GetDeletionTimestamp() != nil {
			deleting = "YES"
		}
		finalizers := strings.Join(obj.GetFinalizers(), ",")

		avail := statusUnknown
		degradedCond := statusUnknown
		prog := statusUnknown

		if deleting == "YES" {
			degraded = append(degraded, degradedAddon{
				cluster: cluster,
				addon:   name,
				detail:  fmt.Sprintf("Terminating with finalizers [%s]", finalizers),
			})
		}

		conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
		if found {
			for _, c := range conditions {
				cMap, ok := c.(map[string]any)
				if !ok {
					continue
				}
				cType, _ := cMap["type"].(string)
				cStatus, _ := cMap["status"].(string)
				cMsg, _ := cMap["message"].(string)
				cReason, _ := cMap["reason"].(string)

				switch cType {
				case conditionAvailable:
					avail = cStatus
				case conditionDegraded:
					degradedCond = cStatus
				case conditionProgressing:
					prog = cStatus
				}

				if (cType == conditionDegraded && cStatus == statusTrue) ||
					(cType == conditionAvailable && cStatus == statusFalse) {
					degraded = append(degraded, degradedAddon{
						cluster: cluster,
						addon:   name,
						detail:  fmt.Sprintf("[%s=%s (%s): %s]", cType, cStatus, cReason, cMsg),
					})
				}
			}
		}

		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t[%s]\n",
			cluster,
			name,
			avail,
			degradedCond,
			prog,
			deleting,
			finalizers,
		)
	}
	_ = writer.Flush()

	if len(degraded) > 0 {
		sb.WriteString("\nDegraded or Terminating ManagedClusterAddOns Details:\n")
		for _, d := range degraded {
			sb.WriteString(fmt.Sprintf("  - %s/%s: %s\n", d.cluster, d.addon, d.detail))
		}
	}

	klog.Info(sb.String())
}

// LogClusterManagementAddOn logs the status and install strategy of ClusterManagementAddOn.
func LogClusterManagementAddOn(client dynamic.Interface) {
	gvr := NewMCOClusterManagementAddonsGVR()
	obj, err := client.Resource(gvr).Get(context.TODO(), "multicluster-observability-addon", metav1.GetOptions{})
	if err != nil {
		klog.V(1).Infof("No ClusterManagementAddOn multicluster-observability-addon found: %v", err)
		return
	}

	cleanUnstructuredForLogging(obj)
	var sb strings.Builder
	sb.WriteString("ClusterManagementAddOn multicluster-observability-addon:\n")
	installStrategy, found, _ := unstructured.NestedMap(obj.Object, "spec", "installStrategy")
	if found {
		strategyType, _, _ := unstructured.NestedString(installStrategy, "type")
		sb.WriteString(fmt.Sprintf("  InstallStrategy: %s\n", strategyType))
	}
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if found {
		sb.WriteString("  Conditions:\n")
		for _, c := range conditions {
			if cMap, ok := c.(map[string]any); ok {
				sb.WriteString(fmt.Sprintf("    %s: %s (%s: %s)\n", cMap["type"], cMap["status"], cMap["reason"], cMap["message"]))
			}
		}
	}
	klog.Info(sb.String())
}

func formatPodsStatuses(pods []corev1.Pod) string {
	var sb strings.Builder
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tSTATUS\tRESTARTS\tAGE")
	for _, pod := range pods {
		var restartCount int32
		if len(pod.Status.ContainerStatuses) > 0 {
			restartCount = pod.Status.ContainerStatuses[0].RestartCount
		}
		age := time.Since(pod.CreationTimestamp.Time).Round(time.Second)
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%d\t%s\n",
			pod.Name,
			pod.Status.Phase,
			restartCount,
			age)
	}
	_ = writer.Flush()
	return sb.String()
}

func printPodsStatuses(pods []corev1.Pod) {
	klog.Info(formatPodsStatuses(pods))
}

func formatDeploymentsStatuses(clientset kubernetes.Interface, namespace string) (string, error) {
	deploymentsClient := clientset.AppsV1().Deployments(namespace)
	deployments, err := deploymentsClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list deployments in namespace %s: %w", namespace, err)
	}

	var sb strings.Builder
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tREADY\tUP-TO-DATE\tAVAILABLE\tAGE")
	for _, deployment := range deployments.Items {
		desired := int32(1)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}
		ready := fmt.Sprintf("%d/%d", deployment.Status.ReadyReplicas, desired)
		age := time.Since(deployment.CreationTimestamp.Time).Round(time.Second)
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%s\n",
			deployment.Name,
			ready,
			deployment.Status.UpdatedReplicas,
			deployment.Status.AvailableReplicas,
			age)
	}
	_ = writer.Flush()
	return sb.String(), nil
}

func printDeploymentsStatuses(clientset kubernetes.Interface, namespace string) {
	out, err := formatDeploymentsStatuses(clientset, namespace)
	if err != nil {
		klog.Errorf("%v", err)
		return
	}
	klog.Info(out)
}

func formatStatefulSetsStatuses(clientset kubernetes.Interface, namespace string) (string, error) {
	statefulSetsClient := clientset.AppsV1().StatefulSets(namespace)
	statefulSets, err := statefulSetsClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list statefulsets in namespace %s: %w", namespace, err)
	}

	var sb strings.Builder
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tREADY\tAGE")
	for _, statefulSet := range statefulSets.Items {
		desired := int32(1)
		if statefulSet.Spec.Replicas != nil {
			desired = *statefulSet.Spec.Replicas
		}
		ready := fmt.Sprintf("%d/%d", statefulSet.Status.ReadyReplicas, desired)
		age := time.Since(statefulSet.CreationTimestamp.Time).Round(time.Second)
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\n",
			statefulSet.Name,
			ready,
			age)
	}
	_ = writer.Flush()
	return sb.String(), nil
}

func printStatefulSetsStatuses(clientset kubernetes.Interface, namespace string) {
	out, err := formatStatefulSetsStatuses(clientset, namespace)
	if err != nil {
		klog.Errorf("%v", err)
		return
	}
	klog.Info(out)
}

func formatDaemonSetsStatuses(clientset kubernetes.Interface, namespace string) (string, error) {
	daemonSetsClient := clientset.AppsV1().DaemonSets(namespace)
	daemonSets, err := daemonSetsClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list daemonsets in namespace %s: %w", namespace, err)
	}

	var sb strings.Builder
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tDESIRED\tCURRENT\tREADY\tAGE")
	for _, daemonSet := range daemonSets.Items {
		age := time.Since(daemonSet.CreationTimestamp.Time).Round(time.Second)
		_, _ = fmt.Fprintf(writer, "%s\t%d\t%d\t%d\t%s\n",
			daemonSet.Name,
			daemonSet.Status.DesiredNumberScheduled,
			daemonSet.Status.CurrentNumberScheduled,
			daemonSet.Status.NumberReady,
			age)
	}
	_ = writer.Flush()
	return sb.String(), nil
}

func printDaemonSetsStatuses(clientset kubernetes.Interface, namespace string) {
	out, err := formatDaemonSetsStatuses(clientset, namespace)
	if err != nil {
		klog.Errorf("%v", err)
		return
	}
	klog.Info(out)
}

func printConfigMapsInNamespace(client kubernetes.Interface, ns string) {
	configMaps, err := client.CoreV1().ConfigMaps(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to get configmaps in namespace %q: %v", ns, err)
		return
	}

	if len(configMaps.Items) == 0 {
		klog.V(1).Infof("No configmaps found in namespace %q", ns)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ConfigMaps in namespace %s (total: %d):\n", ns, len(configMaps.Items)))
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tDATA\tAGE")

	dashboardCount := 0
	for _, configMap := range configMaps.Items {
		if strings.HasPrefix(configMap.Name, "grafana-dashboard-") {
			dashboardCount++
			continue
		}
		age := time.Since(configMap.CreationTimestamp.Time).Round(time.Second)
		_, _ = fmt.Fprintf(writer, "%s\t%d\t%s\n",
			configMap.Name,
			len(configMap.Data),
			age)
	}
	_ = writer.Flush()
	if dashboardCount > 0 {
		sb.WriteString(fmt.Sprintf("  (+ %d grafana-dashboard-* ConfigMaps omitted for brevity)\n", dashboardCount))
	}
	klog.Info(sb.String())
}

func printSecretsInNamespace(client kubernetes.Interface, ns string) {
	secrets, err := client.CoreV1().Secrets(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to get secrets in namespace %q: %v", ns, err)
		return
	}

	if len(secrets.Items) == 0 {
		klog.V(1).Infof("No secrets found in namespace %q", ns)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Secrets in namespace %s (total: %d):\n", ns, len(secrets.Items)))
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tTYPE\tDATA\tAGE")
	for _, secret := range secrets.Items {
		age := time.Since(secret.CreationTimestamp.Time).Round(time.Second)
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%d\t%s\n",
			secret.Name,
			secret.Type,
			len(secret.Data),
			age)
	}
	_ = writer.Flush()
	klog.Info(sb.String())
}

func printAddonDeploymentConfigs(client dynamic.Interface, ns string) {
	gvr := NewMCOAddOnDeploymentConfigGVR()
	objs, err := client.Resource(gvr).Namespace(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("failed to list AddOnDeploymentConfigs in namespace %s: %v", ns, err)
		return
	}

	if len(objs.Items) == 0 {
		klog.V(1).Infof("No AddOnDeploymentConfigs found in namespace %s", ns)
		return
	}

	var adcInfo strings.Builder
	adcInfo.WriteString(fmt.Sprintf("AddOnDeploymentConfigs in namespace %s:\n", ns))
	for _, obj := range objs.Items {
		cleanUnstructuredForLogging(&obj)
		adcInfo.WriteString(ToCompactJSON(obj.Object, "", 0, 3) + "\n")
	}
	klog.Info(adcInfo.String())
}

func printManifestWorks(client dynamic.Interface) {
	gvr := NewOCMManifestworksGVR()
	objs, err := client.Resource(gvr).Namespace("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("failed to list ManifestWorks: %v", err)
		return
	}

	var sb strings.Builder
	sb.WriteString("Observability ManifestWorks:\n")
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAMESPACE\tNAME\tAPPLIED\tAVAILABLE\tDEGRADED\tDELETING\tFINALIZERS")

	type degradedMW struct {
		ns     string
		name   string
		detail string
	}
	var degraded []degradedMW
	count := 0

	for _, obj := range objs.Items {
		name := obj.GetName()
		labels := obj.GetLabels()

		addonLabel := ""
		if labels != nil {
			if val, ok := labels["addon.open-cluster-management.io/addon-name"]; ok {
				addonLabel = val
			} else if val, ok := labels["open-cluster-management.io/addon-name"]; ok {
				addonLabel = val
			}
		}

		isMCOA := strings.Contains(name, "multicluster-observability-addon") ||
			addonLabel == "multicluster-observability-addon"

		isLegacy := strings.HasSuffix(name, "-observability") ||
			name == "endpoint-observability-work" ||
			strings.Contains(name, "observability-controller") ||
			strings.Contains(name, "observability-addon") ||
			strings.Contains(addonLabel, "observability") ||
			(!isMCOA && strings.Contains(name, "observability"))

		if !isLegacy && !isMCOA {
			continue
		}
		count++
		ns := obj.GetNamespace()
		deleting := "No"
		if obj.GetDeletionTimestamp() != nil {
			deleting = "YES"
		}
		finalizers := strings.Join(obj.GetFinalizers(), ",")

		applied := statusUnknown
		available := statusUnknown
		degradedCond := statusUnknown

		if deleting == "YES" {
			degraded = append(degraded, degradedMW{
				ns:     ns,
				name:   name,
				detail: fmt.Sprintf("Terminating with finalizers [%s]", finalizers),
			})
		}

		conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
		if found {
			for _, c := range conditions {
				cMap, ok := c.(map[string]any)
				if !ok {
					continue
				}
				cType, _ := cMap["type"].(string)
				cStatus, _ := cMap["status"].(string)
				cMsg, _ := cMap["message"].(string)
				cReason, _ := cMap["reason"].(string)

				switch cType {
				case conditionApplied:
					applied = cStatus
				case conditionAvailable:
					available = cStatus
				case conditionDegraded:
					degradedCond = cStatus
				}

				if (cType == conditionDegraded && cStatus == statusTrue) ||
					(cType == conditionApplied && cStatus == statusFalse) ||
					(cType == conditionAvailable && cStatus == statusFalse) {
					degraded = append(degraded, degradedMW{
						ns:     ns,
						name:   name,
						detail: fmt.Sprintf("[%s=%s (%s): %s]", cType, cStatus, cReason, cMsg),
					})
				}
			}
		}

		manifests, foundRes, _ := unstructured.NestedSlice(obj.Object, "status", "resourceStatus", "manifests")
		if foundRes {
			for _, m := range manifests {
				mMap, ok := m.(map[string]any)
				if !ok {
					continue
				}
				resMeta, _, _ := unstructured.NestedMap(mMap, "resourceMeta")
				kind, _, _ := unstructured.NestedString(resMeta, "kind")
				resName, _, _ := unstructured.NestedString(resMeta, "name")
				resNs, _, _ := unstructured.NestedString(resMeta, "namespace")

				mConditions, foundConds, _ := unstructured.NestedSlice(mMap, "conditions")
				if foundConds {
					for _, mc := range mConditions {
						mcMap, ok := mc.(map[string]any)
						if !ok {
							continue
						}
						mType, _ := mcMap["type"].(string)
						mStatus, _ := mcMap["status"].(string)
						mMsg, _ := mcMap["message"].(string)
						mReason, _ := mcMap["reason"].(string)

						if (mType == conditionApplied && mStatus == statusFalse) ||
							(mType == conditionAvailable && mStatus == statusFalse) ||
							(mType == conditionDegraded && mStatus == statusTrue) {
							degraded = append(degraded, degradedMW{
								ns:     ns,
								name:   name,
								detail: fmt.Sprintf("Manifest %s %s/%s [%s=%s (%s): %s]", kind, resNs, resName, mType, mStatus, mReason, mMsg),
							})
						}
					}
				}
			}
		}

		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t[%s]\n",
			ns,
			name,
			applied,
			available,
			degradedCond,
			deleting,
			finalizers,
		)
	}
	_ = writer.Flush()

	if count == 0 {
		klog.V(1).Info("No observability ManifestWorks found")
		return
	}

	if len(degraded) > 0 {
		sb.WriteString("\nDegraded or Terminating ManifestWorks Details:\n")
		for _, d := range degraded {
			sb.WriteString(fmt.Sprintf("  - %s/%s: %s\n", d.ns, d.name, d.detail))
		}
	}

	klog.Info(sb.String())
}

// GetDiscoveredManagedClusterNames queries the Hub's ManagedCluster resources and returns their names.
func GetDiscoveredManagedClusterNames(ctx context.Context, client dynamic.Interface) ([]string, error) {
	objs, err := client.Resource(NewOCMManagedClustersGVR()).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list managed clusters: %w", err)
	}
	names := make([]string, 0, len(objs.Items))
	for _, obj := range objs.Items {
		names = append(names, obj.GetName())
	}
	return names, nil
}

func extractKubeconfigFromSecret(s *corev1.Secret) []byte {
	if s == nil || s.Data == nil {
		return nil
	}
	return s.Data["kubeconfig"]
}

// resolveSpokeClientsFromHub attempts to find an admin kubeconfig secret on the Hub for a managed cluster
// and constructs Kubernetes client interfaces for inspecting the spoke.
func resolveSpokeClientsFromHub(
	ctx context.Context,
	hubClient kubernetes.Interface,
	hubDynClient dynamic.Interface,
	clusterName string,
) (kubernetes.Interface, dynamic.Interface, error) {
	var kubeconfigBytes []byte

	// 1. Check Hive ClusterDeployment for explicit adminKubeconfigSecretRef
	cdGVR := NewHiveClusterDeploymentGVR()
	if cdObj, err := hubDynClient.Resource(cdGVR).Namespace(clusterName).Get(ctx, clusterName, metav1.GetOptions{}); err == nil {
		secretName, found, _ := unstructured.NestedString(cdObj.Object, "spec", "clusterMetadata", "adminKubeconfigSecretRef", "name")
		if found && secretName != "" {
			if s, err := hubClient.CoreV1().Secrets(clusterName).Get(ctx, secretName, metav1.GetOptions{}); err == nil {
				kubeconfigBytes = extractKubeconfigFromSecret(s)
			}
		}
	}

	// 2. Check standard admin kubeconfig secret names in cluster namespace
	if len(kubeconfigBytes) == 0 {
		candidates := []string{
			fmt.Sprintf("%s-admin-kubeconfig", clusterName),
			"admin-kubeconfig",
		}
		for _, name := range candidates {
			if s, err := hubClient.CoreV1().Secrets(clusterName).Get(ctx, name, metav1.GetOptions{}); err == nil {
				kubeconfigBytes = extractKubeconfigFromSecret(s)
				if len(kubeconfigBytes) > 0 {
					break
				}
			}
		}
	}

	// 3. Check secrets with hive.openshift.io/secret-type=kubeconfig label or ending with -admin-kubeconfig
	if len(kubeconfigBytes) == 0 {
		if list, err := hubClient.CoreV1().Secrets(clusterName).List(ctx, metav1.ListOptions{}); err == nil {
			for i := range list.Items {
				s := &list.Items[i]
				if (s.Labels != nil && s.Labels["hive.openshift.io/secret-type"] == "kubeconfig") ||
					strings.HasSuffix(s.Name, "-admin-kubeconfig") {
					kubeconfigBytes = extractKubeconfigFromSecret(s)
					if len(kubeconfigBytes) > 0 {
						break
					}
				}
			}
		}
	}

	if len(kubeconfigBytes) == 0 {
		return nil, nil, fmt.Errorf("no admin kubeconfig secret found on hub for cluster %s", clusterName)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build REST config from kubeconfig for cluster %s: %w", clusterName, err)
	}

	// Enforce strict timeout so unresponsive spokes don't hang e2e logging
	restConfig.Timeout = 10 * time.Second

	spokeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create kubernetes client for cluster %s: %w", clusterName, err)
	}

	spokeDynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create dynamic client for cluster %s: %w", clusterName, err)
	}

	return spokeClient, spokeDynClient, nil
}

// logSpokeClusterDebugInfo runs diagnostics on the spoke cluster's observability workloads.
func logSpokeClusterDebugInfo(
	spokeClient kubernetes.Interface,
	spokeDynClient dynamic.Interface,
	clusterName string,
	isMCOA bool,
) {
	// Probe spoke reachability before issuing multiple sequential API calls to avoid wasting timeouts.
	probeNS := MCO_ADDON_NAMESPACE
	if isMCOA {
		probeNS = MCO_AGENT_ADDON_NAMESPACE
	}
	if _, err := spokeClient.CoreV1().Namespaces().Get(context.TODO(), probeNS, metav1.GetOptions{}); err != nil && !apierrors.IsNotFound(err) {
		klog.Warningf("Spoke cluster %s apiserver is not reachable (%v), skipping spoke diagnostics", clusterName, err)
		return
	}

	if isMCOA {
		klog.Infof("%s (MCOA: %s)", SectionSpokeWorkloads, clusterName)
		CheckDeploymentsInNamespace(spokeClient, MCO_AGENT_ADDON_NAMESPACE)
		CheckStatefulSetsInNamespace(spokeClient, MCO_AGENT_ADDON_NAMESPACE)
		CheckPodsInNamespace(spokeClient, MCO_AGENT_ADDON_NAMESPACE, []string{}, map[string]string{})
	} else {
		klog.Infof("%s (Legacy: %s)", SectionSpokeWorkloads, clusterName)
		PrintObject(context.TODO(), spokeDynClient, NewMCOAddonGVR(), MCO_ADDON_NAMESPACE, "observability-addon")
		CheckDeploymentsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		CheckStatefulSetsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		CheckDaemonSetsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		CheckPodsInNamespace(spokeClient, MCO_ADDON_NAMESPACE, []string{"observability-addon"}, map[string]string{})
		printConfigMapsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		printSecretsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
	}
}
