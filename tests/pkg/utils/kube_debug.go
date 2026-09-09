// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
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

var (
	klogSeverityPattern = regexp.MustCompile(`^(?:\S+\s+)?[EFW]\d{4} \d{2}:\d{2}:\d{2}`)
	klogInfoPattern     = regexp.MustCompile(`^(?:\S+\s+)?(?:I\d{4} \d{2}:\d{2}:\d{2}|INFO\s|DEBUG\s)`)
	logLevelInfoPattern = regexp.MustCompile(`(?i)(?:\blevel=(?:info|debug)\b|\blevel="(?:info|debug)"|"level"\s*:\s*"(?:info|debug)")`)
)

const (
	maxContainerLogLineLength     = 1000
	maxTailLines                  = 20
	maxPrecedingErrorLines        = 30
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
	SectionWarningEvents          = "---------- [SECTION: Recent Warning Events] ----------"
	statusUnknown                 = "Unknown"
	statusTrue                    = "True"
	statusFalse                   = "False"
	statusYes                     = "YES"
	statusNo                      = "No"
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
		sb.WriteString(formatMCOCapabilities(capabilities))
	}

	storageConfig, found, _ := unstructured.NestedMap(obj.Object, "spec", "storageConfig")
	if found {
		if mos, ok := storageConfig["metricObjectStorage"].(map[string]any); ok {
			sb.WriteString(fmt.Sprintf("  StorageConfig: metricObjectStorage=%s/%s\n", mos["name"], mos["key"]))
		}
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

// formatMCOCapabilities formats the MCO capabilities spec into clean human-readable lines.
func formatMCOCapabilities(capabilities map[string]any) string {
	var sb strings.Builder
	sb.WriteString("  Capabilities:\n")

	// Platform capabilities
	if platform, ok := capabilities["platform"].(map[string]any); ok {
		var parts []string
		if metrics, ok := platform["metrics"].(map[string]any); ok {
			if def, ok := metrics["default"].(map[string]any); ok {
				if enabled, ok := def["enabled"].(bool); ok {
					parts = append(parts, fmt.Sprintf("metrics=%t", enabled))
				}
			}
			if alerts, ok := metrics["alerts"].(map[string]any); ok {
				if enabled, ok := alerts["enabled"].(bool); ok {
					parts = append(parts, fmt.Sprintf("alerts=%t", enabled))
				}
			}
		}
		if analytics, ok := platform["analytics"].(map[string]any); ok {
			var rs []string
			if ns, ok := analytics["namespaceRightSizingRecommendation"].(map[string]any); ok {
				if enabled, ok := ns["enabled"].(bool); ok {
					rs = append(rs, fmt.Sprintf("ns=%t", enabled))
				}
			}
			if virt, ok := analytics["virtualizationRightSizingRecommendation"].(map[string]any); ok {
				if enabled, ok := virt["enabled"].(bool); ok {
					rs = append(rs, fmt.Sprintf("virt=%t", enabled))
				}
			}
			if len(rs) > 0 {
				parts = append(parts, fmt.Sprintf("rightSizing(%s)", strings.Join(rs, ", ")))
			}
		}
		if len(parts) > 0 {
			sb.WriteString(fmt.Sprintf("    Platform: %s\n", strings.Join(parts, ", ")))
		}
	}

	// UserWorkload capabilities
	if uwl, ok := capabilities["userWorkloads"].(map[string]any); ok {
		var parts []string
		if metrics, ok := uwl["metrics"].(map[string]any); ok {
			if def, ok := metrics["default"].(map[string]any); ok {
				if enabled, ok := def["enabled"].(bool); ok {
					parts = append(parts, fmt.Sprintf("metrics=%t", enabled))
				}
			}
			if alerts, ok := metrics["alerts"].(map[string]any); ok {
				if enabled, ok := alerts["enabled"].(bool); ok {
					parts = append(parts, fmt.Sprintf("alerts=%t", enabled))
				}
			}
		}
		if len(parts) > 0 {
			sb.WriteString(fmt.Sprintf("    UserWorkloads: %s\n", strings.Join(parts, ", ")))
		}
	}

	// Fallback to compact JSON if structure did not yield readable parts
	if sb.Len() == len("  Capabilities:\n") {
		capJSON, _ := json.Marshal(capabilities)
		return fmt.Sprintf("  Capabilities: %s\n", string(capJSON))
	}
	return sb.String()
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
	LogNodes(hubClient, "Hub")
	CheckPodsInNamespace(hubClient, "open-cluster-management", []string{"multicluster-observability-operator"}, map[string]string{
		"name": "multicluster-observability-operator",
	})
	CheckDeploymentsInNamespace(hubClient, MCO_NAMESPACE)
	CheckStatefulSetsInNamespace(hubClient, MCO_NAMESPACE)
	CheckDaemonSetsInNamespace(hubClient, MCO_NAMESPACE)
	CheckJobsInNamespace(hubClient, MCO_NAMESPACE)
	CheckPodsInNamespace(hubClient, MCO_NAMESPACE, []string{"multicluster-observability-addon-manager"}, map[string]string{})
	printConfigMapsInNamespace(hubClient, MCO_NAMESPACE)
	printSecretsInNamespace(hubClient, MCO_NAMESPACE)
	logClusterMonitoringConfigStatus(hubClient, "Hub")

	if isMCOA {
		CheckDeploymentsInNamespace(hubClient, MCO_AGENT_ADDON_NAMESPACE)
		CheckStatefulSetsInNamespace(hubClient, MCO_AGENT_ADDON_NAMESPACE)
		CheckJobsInNamespace(hubClient, MCO_AGENT_ADDON_NAMESPACE)
		CheckPodsInNamespace(hubClient, MCO_AGENT_ADDON_NAMESPACE, []string{"endpoint-monitoring-operator", "prom-agent", "observability-monitoring-cleanup"}, map[string]string{})
		printMCOACustomResources(hubDynClient, MCO_AGENT_ADDON_NAMESPACE)
	}

	// Section 5: Recent Warning Events
	klog.Info(SectionWarningEvents)
	warningNamespaces := []string{"open-cluster-management", MCO_NAMESPACE}
	if isMCOA {
		warningNamespaces = append(warningNamespaces, MCO_AGENT_ADDON_NAMESPACE)
	}
	printRecentWarningEvents(hubClient, warningNamespaces, 20*time.Minute, 20)

	// Section 6: Spoke Clusters
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

// isObservabilityWorkloadInSharedNamespace returns true if a workload (pod, deployment, statefulset, daemonset)
// in MCO_AGENT_ADDON_NAMESPACE belongs to observability or monitoring workloads.
func isObservabilityWorkloadInSharedNamespace(workloadName string) bool {
	for _, prefix := range []string{
		"endpoint-monitoring-operator",
		"observability-monitoring-cleanup",
		"observability-addon",
		"prom-agent",
		"prometheus",
		"alertmanager",
		"metrics-collector",
		"uwl-metrics-collector",
		"node-exporter",
	} {
		if strings.Contains(workloadName, prefix) {
			return true
		}
	}
	return false
}

// isPodAncientFailure returns true if a non-running pod failed or terminated long before
// the current test execution window (e.g., > 1 hour ago).
func isPodAncientFailure(pod corev1.Pod, maxAge time.Duration) bool {
	if pod.CreationTimestamp.IsZero() {
		return false
	}
	if time.Since(pod.CreationTimestamp.Time) < maxAge {
		return false
	}

	for _, cond := range pod.Status.Conditions {
		if !cond.LastTransitionTime.IsZero() && time.Since(cond.LastTransitionTime.Time) < maxAge {
			return false
		}
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil && !cs.State.Terminated.FinishedAt.IsZero() {
			if time.Since(cs.State.Terminated.FinishedAt.Time) < maxAge {
				return false
			}
		}
		if cs.LastTerminationState.Terminated != nil && !cs.LastTerminationState.Terminated.FinishedAt.IsZero() {
			if time.Since(cs.LastTerminationState.Terminated.FinishedAt.Time) < maxAge {
				return false
			}
		}
	}

	return true
}

// getPodWorkloadKey returns the controller name or workload prefix of a pod to identify
// duplicate replica failures (e.g. "metrics-collector-deployment-bf4cc564f").
func getPodWorkloadKey(pod corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			return ref.Name
		}
	}
	// Fallback to stripping the random suffix if the pod name matches standard K8s naming (<workload>-<random>)
	if idx := strings.LastIndex(pod.Name, "-"); idx > 0 {
		return pod.Name[:idx]
	}
	return pod.Name
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

	// In shared agent namespace, filter to observability workloads only to avoid noise
	// from unrelated agent addons (e.g. hypershift-addon-agent, cluster-proxy, klusterlet).
	if ns == MCO_AGENT_ADDON_NAMESPACE {
		var scopedPods []corev1.Pod
		for _, pod := range pods.Items {
			if isObservabilityWorkloadInSharedNamespace(pod.Name) {
				scopedPods = append(scopedPods, pod)
			}
		}
		pods.Items = scopedPods
	}

	if len(pods.Items) == 0 {
		klog.V(1).Infof("No pods found in namespace %s", ns)
		return
	}

	klog.V(1).Infof("Checking %d pods in namespace %q", len(pods.Items), ns)
	printPodsStatuses(pods.Items)

	const maxFailedPodsPerWorkload = 2
	notRunningPodsCount := 0
	activeUnhealthyPodsCount := 0
	skippedAncientPodsCount := 0
	forcedPodsLogged := make(map[string]bool)
	failedPodsLoggedPerWorkload := make(map[string]int)
	skippedFailedPodsCount := make(map[string]int)

	workloadHasRunningPod := make(map[string]bool)
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			workloadHasRunningPod[getPodWorkloadKey(pod)] = true
		}
	}

	for _, pod := range pods.Items {
		force := false
		for _, forcePodName := range forcePodNamesLog {
			if strings.Contains(pod.Name, forcePodName) {
				force = true
				break
			}
		}

		// In shared agent namespace, skip deep inspection of pods belonging to other ACM addons unless forced
		if ns == MCO_AGENT_ADDON_NAMESPACE && !isObservabilityWorkloadInSharedNamespace(pod.Name) && !force {
			continue
		}

		isRunningOrSucceeded := pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodSucceeded

		if !isRunningOrSucceeded {
			// Skip deep diagnostics and failure counting for ancient dead pods (> 1h) from previous runs.
			// Ancient failures should never be dumped or counted against current test runs, even if matching forcePodNamesLog.
			if isPodAncientFailure(pod, 1*time.Hour) {
				klog.V(2).Infof("Skipping deep diagnostics for ancient failed pod %s/%s", ns, pod.Name)
				skippedAncientPodsCount++
				continue
			}
			notRunningPodsCount++

			workloadKey := getPodWorkloadKey(pod)
			if !workloadHasRunningPod[workloadKey] {
				activeUnhealthyPodsCount++
			}

			if !force {
				if failedPodsLoggedPerWorkload[workloadKey] >= maxFailedPodsPerWorkload {
					skippedFailedPodsCount[workloadKey]++
					continue
				}
				failedPodsLoggedPerWorkload[workloadKey]++
			}
		}

		// For forced pods that are already running, track if we already logged them to avoid duplicates
		if force && isRunningOrSucceeded {
			alreadyLogged := false
			for _, forcePodName := range forcePodNamesLog {
				if strings.Contains(pod.Name, forcePodName) {
					if forcedPodsLogged[forcePodName] {
						alreadyLogged = true
					} else {
						forcedPodsLogged[forcePodName] = true
					}
					break
				}
			}
			if alreadyLogged {
				force = false
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

	if len(skippedFailedPodsCount) > 0 {
		workloads := make([]string, 0, len(skippedFailedPodsCount))
		for w := range skippedFailedPodsCount {
			workloads = append(workloads, w)
		}
		slices.Sort(workloads)
		for _, w := range workloads {
			klog.Infof("Skipped deep diagnostics for %d additional failed/evicted pod(s) belonging to %q (already logged %d sample(s))",
				skippedFailedPodsCount[w], w, maxFailedPodsPerWorkload)
		}
	}

	summary, isError := formatNamespacePodHealthSummary(ns, notRunningPodsCount, activeUnhealthyPodsCount, skippedAncientPodsCount)
	switch {
	case isError:
		klog.Error(summary)
	case notRunningPodsCount > 0:
		klog.Info(summary)
	default:
		klog.V(1).Info(summary)
	}
}

// formatNamespacePodHealthSummary produces an informative health message for pods in a namespace,
// differentiating between active workload outages (no running replicas) and historical evicted/terminated pods.
func formatNamespacePodHealthSummary(ns string, notRunningCount, activeUnhealthyCount, skippedAncientCount int) (string, bool) {
	if activeUnhealthyCount > 0 {
		if skippedAncientCount > 0 {
			return fmt.Sprintf("Found %d active unhealthy pod(s) without running replicas in namespace %q (%d ancient failed pod(s) skipped)",
				activeUnhealthyCount, ns, skippedAncientCount), true
		}
		return fmt.Sprintf("Found %d active unhealthy pod(s) without running replicas in namespace %q", activeUnhealthyCount, ns), true
	}
	if notRunningCount > 0 {
		if skippedAncientCount > 0 {
			return fmt.Sprintf("All active workloads are running in namespace %q (%d historical evicted/failed pod(s) with active replacements, %d ancient failed pod(s) skipped)",
				ns, notRunningCount, skippedAncientCount), false
		}
		return fmt.Sprintf("All active workloads are running in namespace %q (%d historical evicted/failed pod(s) with active replacements)",
			ns, notRunningCount), false
	}
	if skippedAncientCount > 0 {
		return fmt.Sprintf("All active pods are healthy in namespace %q (%d ancient failed pod(s) skipped)", ns, skippedAncientCount), false
	}
	return fmt.Sprintf("All pods are running in namespace %q", ns), false
}

func formatContainerState(state corev1.ContainerState) string {
	if state.Running != nil {
		if !state.Running.StartedAt.IsZero() {
			return fmt.Sprintf("Running (started %s)", state.Running.StartedAt.Format("2006-01-02 15:04:05"))
		}
		return "Running"
	}
	if state.Waiting != nil {
		if state.Waiting.Message != "" {
			return fmt.Sprintf("Waiting (%s: %s)", state.Waiting.Reason, state.Waiting.Message)
		}
		if state.Waiting.Reason != "" {
			return fmt.Sprintf("Waiting (%s)", state.Waiting.Reason)
		}
		return "Waiting"
	}
	if state.Terminated != nil {
		return formatTerminatedState(state.Terminated)
	}
	return "Unknown"
}

func formatTerminatedState(term *corev1.ContainerStateTerminated) string {
	msg := ""
	if term.Message != "" {
		msg = fmt.Sprintf(": %s", term.Message)
	}
	return fmt.Sprintf("Terminated (exit code %d, reason: %s%s)", term.ExitCode, term.Reason, msg)
}

func formatPodStatus(pod corev1.Pod) string {
	var podStatus strings.Builder
	podStatus.WriteString(">>>>>>>>>> pod status >>>>>>>>>>\n")
	podStatus.WriteString("Conditions:\n")
	for _, condition := range pod.Status.Conditions {
		details := ""
		switch {
		case condition.Reason != "" && condition.Message != "":
			details = fmt.Sprintf(" (%s: %s)", condition.Reason, condition.Message)
		case condition.Reason != "":
			details = fmt.Sprintf(" (%s)", condition.Reason)
		case condition.Message != "":
			details = fmt.Sprintf(" (%s)", condition.Message)
		}
		podStatus.WriteString(fmt.Sprintf("\t%s: %s%s %v\n", condition.Type, condition.Status, details, condition.LastTransitionTime.Time))
	}
	if len(pod.Status.ContainerStatuses) > 0 {
		podStatus.WriteString("ContainerStatuses:\n")
		for _, cs := range pod.Status.ContainerStatuses {
			podStatus.WriteString(fmt.Sprintf("\t- %s: Ready=%t, Restarts=%d, State: %s\n",
				cs.Name, cs.Ready, cs.RestartCount, formatContainerState(cs.State)))
			if cs.LastTerminationState.Terminated != nil {
				podStatus.WriteString(fmt.Sprintf("\t  Last State: %s\n",
					formatTerminatedState(cs.LastTerminationState.Terminated)))
			}
		}
	}
	if len(pod.Status.InitContainerStatuses) > 0 {
		podStatus.WriteString("InitContainerStatuses:\n")
		for _, cs := range pod.Status.InitContainerStatuses {
			podStatus.WriteString(fmt.Sprintf("\t- %s: Ready=%t, Restarts=%d, State: %s\n",
				cs.Name, cs.Ready, cs.RestartCount, formatContainerState(cs.State)))
			if cs.LastTerminationState.Terminated != nil {
				podStatus.WriteString(fmt.Sprintf("\t  Last State: %s\n",
					formatTerminatedState(cs.LastTerminationState.Terminated)))
			}
		}
	}
	podStatus.WriteString("<<<<<<<<<< pod status <<<<<<<<<<")
	return podStatus.String()
}

func LogPodStatus(pod corev1.Pod) {
	klog.V(1).Infof("Pod %q is in phase %q and status: \n%s", pod.Name, pod.Status.Phase, formatPodStatus(pod))
}

// isErrorLine identifies error, fatal, panic, failure, or timeout signatures.
func isErrorLine(line string) bool {
	lower := strings.ToLower(line)
	// Panic signatures always take precedence regardless of logger prefix
	if strings.Contains(lower, "panic") {
		return true
	}
	// Suppress explicit info/debug lines (e.g. klog I0909, Zap INFO, or logfmt/JSON level=info/debug)
	// that may contain words like "failed" or "error" in routine condition/status updates.
	if klogInfoPattern.MatchString(line) || logLevelInfoPattern.MatchString(line) {
		return false
	}
	// Suppress routine Prometheus federate scrape warnings
	if strings.Contains(lower, "error on ingesting out-of-order samples") {
		return false
	}
	// Suppress standard client-go and controller-runtime in-cluster startup warnings
	if strings.Contains(lower, "neither --kubeconfig nor --master was specified") ||
		strings.Contains(lower, "authorization is disabled") ||
		strings.Contains(lower, "authentication is disabled") {
		return false
	}
	if strings.Contains(lower, "error") ||
		strings.Contains(lower, "fatal") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "exception") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "timed out") {
		return true
	}
	return klogSeverityPattern.MatchString(line)
}

func LogPodLogs(client kubernetes.Interface, ns string, pod corev1.Pod) {
	if pod.Status.Phase == corev1.PodPending {
		// Containers have not started; logs are not available
		return
	}
	if pod.Status.Reason == "Evicted" {
		klog.V(2).Infof("Pod %s was evicted, container logs are not available", pod.Name)
		return
	}

	for _, container := range pod.Spec.Containers {
		// Most e2e tests have a 5-minute timeout; a 6-minute window ensures we capture
		// all relevant logs and errors from the failing test assertion window.
		sinceSeconds := int64(360)
		limitBytes := int64(5 * 1024 * 1024)
		logsRes := client.CoreV1().Pods(ns).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container:    container.Name,
			Timestamps:   true,
			SinceSeconds: &sinceSeconds,
			LimitBytes:   &limitBytes,
		}).Do(context.Background())

		if logsRes.Error() != nil {
			errStr := logsRes.Error().Error()
			if strings.Contains(errStr, "is terminated") ||
				strings.Contains(errStr, "waiting to start") ||
				strings.Contains(errStr, "ContainerCreating") ||
				strings.Contains(errStr, "not found") {
				klog.V(2).Infof("Logs unavailable for pod %q container %q: %s", pod.Name, container.Name, errStr)
			} else {
				klog.Errorf("Failed to get logs for pod %q container %q: %s", pod.Name, container.Name, errStr)
			}
			continue
		}

		logs, err := logsRes.Raw()
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "is terminated") ||
				strings.Contains(errStr, "waiting to start") ||
				strings.Contains(errStr, "ContainerCreating") ||
				strings.Contains(errStr, "not found") {
				klog.V(2).Infof("Logs unavailable for pod %q container %q: %s", pod.Name, container.Name, errStr)
			} else {
				klog.Errorf("Failed to get logs for pod %q container %q: %s", pod.Name, container.Name, errStr)
			}
			continue
		}

		cutoffTime := time.Now().Add(-6 * time.Minute)
		displayLines, msg := formatContainerLogs(string(logs), cutoffTime)

		if len(displayLines) > 0 {
			delimitedLogs := fmt.Sprintf(">>>>>>>>>> container logs: %s/%s >>>>>>>>>>\n%s\n<<<<<<<<<< container logs: %s/%s <<<<<<<<<<",
				pod.Name, container.Name, strings.Join(displayLines, "\n"), pod.Name, container.Name)
			klog.V(1).Infof("Pod %q container %q logs (%s): \n%s", pod.Name, container.Name, msg, delimitedLogs)
		}
	}
}

// truncateLogLine caps individual log lines to maxLen characters to prevent massive query
// URLs (e.g. Prometheus federate scrape parameters) from overwhelming debug logs.
// It ensures truncation occurs on a valid UTF-8 rune boundary.
func truncateLogLine(line string, maxLen int) string {
	if len(line) <= maxLen {
		return line
	}
	for maxLen > 0 && !utf8.RuneStart(line[maxLen]) {
		maxLen--
	}
	return line[:maxLen] + " ... [truncated]"
}

// formatContainerLogs parses raw container logs, captures the most recent tail lines
// (last 20 lines) plus any preceding error/warning lines within the cutoff window (past 6m),
// and prepends omission notices if earlier lines were filtered.
func formatContainerLogs(rawLogs string, cutoffTime time.Time) ([]string, string) {
	lines := strings.Split(rawLogs, "\n")
	var windowLines []string
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
	}

	if len(windowLines) == 0 {
		return nil, "no logs found in 6m window"
	}

	// Reverse windowLines so they are in chronological order (oldest to newest)
	slices.Reverse(windowLines)

	for i, l := range windowLines {
		windowLines[i] = truncateLogLine(l, maxContainerLogLineLength)
	}

	// If all lines fit within the tail limit, return them all directly
	if len(windowLines) <= maxTailLines {
		msg := fmt.Sprintf("all %d log lines from past 6 minutes", len(windowLines))
		return windowLines, msg
	}

	// Split into preceding lines and tail lines
	splitIdx := len(windowLines) - maxTailLines
	precedingLines := windowLines[:splitIdx]
	tailLines := windowLines[splitIdx:]

	// Extract error lines from preceding lines
	var precedingErrors []string
	for _, l := range precedingLines {
		if isErrorLine(l) {
			precedingErrors = append(precedingErrors, l)
		}
	}

	var displayLines []string
	var msg string

	if len(precedingErrors) > 0 {
		omittedPrecedingErrors := 0
		if len(precedingErrors) > maxPrecedingErrorLines {
			omittedPrecedingErrors = len(precedingErrors) - maxPrecedingErrorLines
			precedingErrors = precedingErrors[omittedPrecedingErrors:]
		}
		omittedNonErrors := len(precedingLines) - (len(precedingErrors) + omittedPrecedingErrors)

		var notice string
		switch {
		case omittedPrecedingErrors > 0 && omittedNonErrors > 0:
			notice = fmt.Sprintf("  (+ %d older error lines and %d non-error lines omitted for brevity)", omittedPrecedingErrors, omittedNonErrors)
		case omittedPrecedingErrors > 0:
			notice = fmt.Sprintf("  (+ %d older error lines omitted for brevity)", omittedPrecedingErrors)
		case omittedNonErrors > 0:
			notice = fmt.Sprintf("  (+ %d earlier non-error lines omitted for brevity)", omittedNonErrors)
		}

		if notice != "" {
			displayLines = append(displayLines, notice)
		}
		displayLines = append(displayLines, precedingErrors...)
		displayLines = append(displayLines, "  --- [tail: last 20 log lines] ---")
		displayLines = append(displayLines, tailLines...)
		msg = fmt.Sprintf("last %d log lines + %d preceding error/warning line(s) from past 6 minutes", len(tailLines), len(precedingErrors))
	} else {
		notice := fmt.Sprintf("  (+ %d older log lines omitted for brevity; no errors detected in preceding 6m window)", len(precedingLines))
		displayLines = append(displayLines, notice)
		displayLines = append(displayLines, tailLines...)
		msg = fmt.Sprintf("last %d log lines (no preceding errors detected in 6m window)", len(tailLines))
	}

	return displayLines, msg
}

func CheckDeploymentsInNamespace(client kubernetes.Interface, ns string) {
	deployments, err := client.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to get deployments in namespace %s: %v", ns, err)
		return
	}

	if ns == MCO_AGENT_ADDON_NAMESPACE {
		var scoped []appsv1.Deployment
		for _, dep := range deployments.Items {
			if isObservabilityWorkloadInSharedNamespace(dep.Name) {
				scoped = append(scoped, dep)
			}
		}
		deployments.Items = scoped
	}

	if len(deployments.Items) == 0 {
		klog.V(1).Infof("No deployments found in namespace %q", ns)
		return
	}

	klog.V(1).Infof("Deployments in namespace %s:\n", ns)
	printDeploymentsStatuses(client, ns)

	for _, deployment := range deployments.Items {
		// In shared agent namespace, skip deep inspection of deployments belonging to other ACM addons
		if ns == MCO_AGENT_ADDON_NAMESPACE && !isObservabilityWorkloadInSharedNamespace(deployment.Name) {
			continue
		}

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

	if ns == MCO_AGENT_ADDON_NAMESPACE {
		var scoped []appsv1.StatefulSet
		for _, ss := range statefulSets.Items {
			if isObservabilityWorkloadInSharedNamespace(ss.Name) {
				scoped = append(scoped, ss)
			}
		}
		statefulSets.Items = scoped
	}

	if len(statefulSets.Items) == 0 {
		klog.V(1).Infof("No statefulsets found in namespace %q", ns)
		return
	}

	klog.V(1).Infof("StatefulSets in namespace %s:\n", ns)
	printStatefulSetsStatuses(client, ns)

	for _, statefulSet := range statefulSets.Items {
		// In shared agent namespace, skip deep inspection of statefulsets belonging to other ACM addons
		if ns == MCO_AGENT_ADDON_NAMESPACE && !isObservabilityWorkloadInSharedNamespace(statefulSet.Name) {
			continue
		}

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

	if ns == MCO_AGENT_ADDON_NAMESPACE {
		var scoped []appsv1.DaemonSet
		for _, ds := range daemonSets.Items {
			if isObservabilityWorkloadInSharedNamespace(ds.Name) {
				scoped = append(scoped, ds)
			}
		}
		daemonSets.Items = scoped
	}

	if len(daemonSets.Items) == 0 {
		klog.V(1).Infof("No daemonsets found in namespace %q", ns)
		return
	}

	klog.V(1).Infof("DaemonSets in namespace %s:\n", ns)
	printDaemonSetsStatuses(client, ns)

	for _, daemonSet := range daemonSets.Items {
		// In shared agent namespace, skip deep inspection of daemonsets belonging to other ACM addons
		if ns == MCO_AGENT_ADDON_NAMESPACE && !isObservabilityWorkloadInSharedNamespace(daemonSet.Name) {
			continue
		}
		if daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled &&
			daemonSet.Status.UpdatedNumberScheduled == daemonSet.Status.DesiredNumberScheduled {
			continue
		}

		LogObjectEvents(client, ns, "DaemonSet", daemonSet.Name)
	}
}

func CheckJobsInNamespace(client kubernetes.Interface, ns string) {
	if client == nil {
		return
	}
	jobs, err := client.BatchV1().Jobs(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			klog.Errorf("Failed to get jobs in namespace %s: %v", ns, err)
		}
		return
	}

	if ns == MCO_AGENT_ADDON_NAMESPACE {
		var scoped []batchv1.Job
		for _, j := range jobs.Items {
			if isObservabilityWorkloadInSharedNamespace(j.Name) {
				scoped = append(scoped, j)
			}
		}
		jobs.Items = scoped
	}

	if len(jobs.Items) == 0 {
		klog.V(1).Infof("No jobs found in namespace %q", ns)
		return
	}

	klog.V(1).Infof("Jobs in namespace %s:\n", ns)
	printJobsStatuses(client, ns)

	for _, job := range jobs.Items {
		// In shared agent namespace, skip deep inspection of jobs belonging to other ACM addons
		if ns == MCO_AGENT_ADDON_NAMESPACE && !isObservabilityWorkloadInSharedNamespace(job.Name) {
			continue
		}

		isComplete := false
		for _, c := range job.Status.Conditions {
			if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
				isComplete = true
				break
			}
		}
		if isComplete {
			continue
		}

		LogObjectEvents(client, ns, "Job", job.Name)
	}
}

func getEventTimestamp(event corev1.Event) time.Time {
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	if !event.FirstTimestamp.IsZero() {
		return event.FirstTimestamp.Time
	}
	return time.Time{}
}

func formatObjectEvents(kind string, events []corev1.Event) string {
	if len(events) == 0 {
		return ""
	}

	// Sort events chronologically by last timestamp / event time
	sortedEvents := make([]corev1.Event, len(events))
	copy(sortedEvents, events)
	slices.SortFunc(sortedEvents, func(a, b corev1.Event) int {
		ta := getEventTimestamp(a)
		tb := getEventTimestamp(b)
		return ta.Compare(tb)
	})

	// Filter out events older than 1 hour to keep diagnostics relevant to recent test activity
	cutoff := time.Now().Add(-1 * time.Hour)
	recentEvents := make([]corev1.Event, 0, len(sortedEvents))
	staleEventsCount := 0
	for _, event := range sortedEvents {
		ts := getEventTimestamp(event)
		if !ts.IsZero() && ts.Before(cutoff) {
			staleEventsCount++
			continue
		}
		recentEvents = append(recentEvents, event)
	}

	if len(recentEvents) == 0 {
		return ""
	}

	const maxEventsToDisplay = 25
	itemsToDisplay := recentEvents
	omittedCount := staleEventsCount
	if len(recentEvents) > maxEventsToDisplay {
		omittedCount += len(recentEvents) - maxEventsToDisplay
		itemsToDisplay = recentEvents[len(recentEvents)-maxEventsToDisplay:]
	}

	objectEvents := make([]string, 0, len(itemsToDisplay)+1)
	if omittedCount > 0 {
		objectEvents = append(objectEvents, fmt.Sprintf("  (+ %d older events omitted for brevity)", omittedCount))
	}
	for _, event := range itemsToDisplay {
		ts := getEventTimestamp(event)
		tsStr := "unknown"
		if !ts.IsZero() {
			tsStr = ts.Format("2006-01-02 15:04:05 -0700 MST")
		}
		count := event.Count
		if count == 0 {
			count = 1
		}
		objectEvents = append(objectEvents, fmt.Sprintf("%s %s (%d): %s", event.Reason, tsStr, count, event.Message))
	}
	return fmt.Sprintf(">>>>>>>>>> %s events >>>>>>>>>>\n%s\n<<<<<<<<<< %s events <<<<<<<<<<", kind, strings.Join(objectEvents, "\n"), kind)
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

	if len(events.Items) == 0 {
		return
	}

	formattedEvents := formatObjectEvents(kind, events.Items)
	if formattedEvents == "" {
		return
	}
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

// shortHash returns a truncated 8-character hash representation, or "<empty>" if hash is empty.
func shortHash(h string) string {
	if h == "" {
		return "<empty>"
	}
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

// formatManagedClusterAddOns formats the ManagedClusterAddOn resources table and degraded details,
// including abnormal conditions and individual mismatched configReferences.
func formatManagedClusterAddOns(items []unstructured.Unstructured) string {
	var sb strings.Builder
	sb.WriteString("ManagedClusterAddOns:\n")
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "CLUSTER\tADDON\tAVAILABLE\tDEGRADED\tPROGRESSING\tDELETING\tFINALIZERS\tCONFIGS")

	type degradedAddon struct {
		cluster string
		addon   string
		detail  string
	}
	var degraded []degradedAddon
	count := 0

	for _, obj := range items {
		name := obj.GetName()
		if !strings.Contains(name, "observability") {
			continue
		}
		count++
		cluster := obj.GetNamespace()
		deleting := statusNo
		if obj.GetDeletionTimestamp() != nil {
			deleting = statusYes
		}
		finalizers := strings.Join(obj.GetFinalizers(), ",")

		avail := statusUnknown
		degradedCond := statusUnknown
		prog := statusUnknown

		if deleting == statusYes {
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
					(cType == conditionAvailable && (cStatus == statusFalse || cStatus == statusUnknown)) ||
					(cType == conditionProgressing && cStatus == statusTrue && !strings.Contains(cMsg, "completed with no errors")) ||
					(cStatus == statusFalse && cType != conditionProgressing && cType != conditionDegraded) {
					degraded = append(degraded, degradedAddon{
						cluster: cluster,
						addon:   name,
						detail:  fmt.Sprintf("[%s=%s (%s): %s]", cType, cStatus, cReason, cMsg),
					})
				}
			}
		}

		// Inspect individual status.configReferences
		configRefs, foundConfigs, _ := unstructured.NestedSlice(obj.Object, "status", "configReferences")
		var configMismatches []string
		totalConfigs := len(configRefs)
		if foundConfigs && totalConfigs > 0 {
			for _, cr := range configRefs {
				crMap, ok := cr.(map[string]any)
				if !ok {
					continue
				}
				group, _, _ := unstructured.NestedString(crMap, "group")
				resource, _, _ := unstructured.NestedString(crMap, "resource")
				resIdentifier := resource
				if group != "" {
					resIdentifier = resource + "." + group
				}

				desired, hasDesired, _ := unstructured.NestedMap(crMap, "desiredConfig")
				applied, hasApplied, _ := unstructured.NestedMap(crMap, "lastAppliedConfig")

				desName, _, _ := unstructured.NestedString(desired, "name")
				desNs, _, _ := unstructured.NestedString(desired, "namespace")
				desHash, _, _ := unstructured.NestedString(desired, "specHash")

				appldName, _, _ := unstructured.NestedString(applied, "name")
				appldNs, _, _ := unstructured.NestedString(applied, "namespace")
				appldHash, _, _ := unstructured.NestedString(applied, "specHash")

				resName := desName
				if resName == "" {
					resName = appldName
				}
				resNs := desNs
				if resNs == "" {
					resNs = appldNs
				}
				target := resName
				if resNs != "" {
					target = fmt.Sprintf("%s/%s", resNs, resName)
				}
				if target == "" {
					target = "<unnamed>"
				}

				switch {
				case !hasDesired && !hasApplied:
					// nothing to compare
				case hasDesired && !hasApplied:
					detail := fmt.Sprintf("ConfigReference mismatch: %s %s (desiredHash: %s, applied: <none>)",
						resIdentifier, target, shortHash(desHash))
					configMismatches = append(configMismatches, detail)
					degraded = append(degraded, degradedAddon{cluster: cluster, addon: name, detail: detail})
				case !hasDesired && hasApplied:
					detail := fmt.Sprintf("ConfigReference mismatch: %s %s (desired: <none>, appliedHash: %s)",
						resIdentifier, target, shortHash(appldHash))
					configMismatches = append(configMismatches, detail)
					degraded = append(degraded, degradedAddon{cluster: cluster, addon: name, detail: detail})
				case desName != appldName || desNs != appldNs:
					detail := fmt.Sprintf("ConfigReference mismatch: %s (desired: %s/%s, applied: %s/%s)",
						resIdentifier, desNs, desName, appldNs, appldName)
					configMismatches = append(configMismatches, detail)
					degraded = append(degraded, degradedAddon{cluster: cluster, addon: name, detail: detail})
				case desHash != appldHash:
					detail := fmt.Sprintf("ConfigReference mismatch: %s %s (desiredHash: %s, appliedHash: %s)",
						resIdentifier, target, shortHash(desHash), shortHash(appldHash))
					configMismatches = append(configMismatches, detail)
					degraded = append(degraded, degradedAddon{cluster: cluster, addon: name, detail: detail})
				}
			}
		}

		configsSummary := "-"
		if totalConfigs > 0 {
			if len(configMismatches) > 0 {
				configsSummary = fmt.Sprintf("[%d/%d sync, %d mismatch]", totalConfigs-len(configMismatches), totalConfigs, len(configMismatches))
			} else {
				configsSummary = fmt.Sprintf("[%d/%d sync]", totalConfigs, totalConfigs)
			}
		}

		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t[%s]\t%s\n",
			cluster,
			name,
			avail,
			degradedCond,
			prog,
			deleting,
			finalizers,
			configsSummary,
		)
	}
	_ = writer.Flush()

	if count == 0 {
		return ""
	}

	if len(degraded) > 0 {
		sb.WriteString("\nDegraded or Terminating ManagedClusterAddOns Details:\n")
		for _, d := range degraded {
			detail := truncateLogLine(d.detail, 400)
			sb.WriteString(fmt.Sprintf("  - %s/%s: %s\n", d.cluster, d.addon, detail))
		}
	}

	return sb.String()
}

// LogManagedClusterAddOns lists and displays status of ManagedClusterAddOn resources across all clusters.
func LogManagedClusterAddOns(client dynamic.Interface) {
	gvr := NewMCOManagedClusterAddonsGVR()
	objs, err := client.Resource(gvr).Namespace("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to list ManagedClusterAddOns: %v", err)
		return
	}

	output := formatManagedClusterAddOns(objs.Items)
	if output == "" {
		klog.V(1).Info("No observability ManagedClusterAddOns found")
		return
	}
	klog.Info(output)
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
	ancientSucceededCount := 0
	const maxFailedRowsPerWorkload = 2
	failedRowsLoggedPerWorkload := make(map[string]int)
	omittedFailedPodsPerWorkload := make(map[string]int)

	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded && !pod.CreationTimestamp.IsZero() && time.Since(pod.CreationTimestamp.Time) > 1*time.Hour {
			ancientSucceededCount++
			continue
		}

		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
			wKey := getPodWorkloadKey(pod)
			if failedRowsLoggedPerWorkload[wKey] >= maxFailedRowsPerWorkload {
				omittedFailedPodsPerWorkload[wKey]++
				continue
			}
			failedRowsLoggedPerWorkload[wKey]++
		}

		var restartCount int32
		for _, cs := range pod.Status.ContainerStatuses {
			restartCount += cs.RestartCount
		}
		for _, ics := range pod.Status.InitContainerStatuses {
			restartCount += ics.RestartCount
		}
		age := time.Since(pod.CreationTimestamp.Time).Round(time.Second)
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%d\t%s\n",
			pod.Name,
			pod.Status.Phase,
			restartCount,
			age)
	}
	_ = writer.Flush()
	if ancientSucceededCount > 0 {
		_, _ = fmt.Fprintf(&sb, "  (+ %d ancient Succeeded pods omitted for brevity)\n", ancientSucceededCount)
	}
	if len(omittedFailedPodsPerWorkload) > 0 {
		workloads := slices.Sorted(maps.Keys(omittedFailedPodsPerWorkload))
		for _, w := range workloads {
			_, _ = fmt.Fprintf(&sb, "  (+ %d additional non-running pod(s) for %q omitted for brevity)\n",
				omittedFailedPodsPerWorkload[w], w)
		}
	}
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
		if namespace == MCO_AGENT_ADDON_NAMESPACE && !isObservabilityWorkloadInSharedNamespace(deployment.Name) {
			continue
		}
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
		if namespace == MCO_AGENT_ADDON_NAMESPACE && !isObservabilityWorkloadInSharedNamespace(statefulSet.Name) {
			continue
		}
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
		if namespace == MCO_AGENT_ADDON_NAMESPACE && !isObservabilityWorkloadInSharedNamespace(daemonSet.Name) {
			continue
		}
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

func formatJobsStatuses(clientset kubernetes.Interface, namespace string) (string, error) {
	if clientset == nil {
		return "", fmt.Errorf("clientset is nil")
	}
	jobsClient := clientset.BatchV1().Jobs(namespace)
	jobs, err := jobsClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list jobs in namespace %s: %w", namespace, err)
	}

	var sb strings.Builder
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tCOMPLETIONS\tDURATION\tAGE\tCONDITIONS")

	type failedJob struct {
		name   string
		detail string
	}
	var failedJobs []failedJob

	for _, job := range jobs.Items {
		if namespace == MCO_AGENT_ADDON_NAMESPACE && !isObservabilityWorkloadInSharedNamespace(job.Name) {
			continue
		}
		age := time.Since(job.CreationTimestamp.Time).Round(time.Second)
		completionsNeeded := int32(1)
		if job.Spec.Completions != nil {
			completionsNeeded = *job.Spec.Completions
		}
		completions := fmt.Sprintf("%d/%d", job.Status.Succeeded, completionsNeeded)
		duration := "-"
		if job.Status.StartTime != nil {
			if job.Status.CompletionTime != nil {
				duration = job.Status.CompletionTime.Sub(job.Status.StartTime.Time).Round(time.Second).String()
			} else {
				duration = time.Since(job.Status.StartTime.Time).Round(time.Second).String()
			}
		}
		var conds []string
		hasFailureCond := false
		for _, c := range job.Status.Conditions {
			if c.Status == corev1.ConditionTrue {
				conds = append(conds, string(c.Type))
				if c.Type == batchv1.JobFailed || c.Type == batchv1.JobFailureTarget {
					hasFailureCond = true
					reason := c.Reason
					if reason == "" {
						reason = "Unknown"
					}
					msg := strings.TrimSpace(c.Message)
					if msg != "" {
						failedJobs = append(failedJobs, failedJob{
							name:   job.Name,
							detail: fmt.Sprintf("[%s (%s): %s]", c.Type, reason, msg),
						})
					} else {
						failedJobs = append(failedJobs, failedJob{
							name:   job.Name,
							detail: fmt.Sprintf("[%s (%s)]", c.Type, reason),
						})
					}
				}
			}
		}
		if job.Status.Failed > 0 && !hasFailureCond {
			failedJobs = append(failedJobs, failedJob{
				name:   job.Name,
				detail: fmt.Sprintf("[Failed pods: %d]", job.Status.Failed),
			})
		}
		condStr := strings.Join(conds, ",")
		if condStr == "" {
			condStr = "-"
		}
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			job.Name,
			completions,
			duration,
			age,
			condStr)
	}
	_ = writer.Flush()

	if len(failedJobs) > 0 {
		sb.WriteString("\nFailed or Degraded Jobs Details:\n")
		for _, fj := range failedJobs {
			detail := truncateLogLine(fj.detail, 400)
			sb.WriteString(fmt.Sprintf("  - %s/%s: %s\n", namespace, fj.name, detail))
		}
	}
	return sb.String(), nil
}

func printJobsStatuses(clientset kubernetes.Interface, namespace string) {
	out, err := formatJobsStatuses(clientset, namespace)
	if err != nil {
		klog.Errorf("%v", err)
		return
	}
	klog.Info(out)
}

func formatNodesStatuses(clientset kubernetes.Interface) (string, error) {
	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return "No nodes found", nil
	}

	var sb strings.Builder
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tSTATUS\tROLES\tAGE\tPRESSURE/ISSUES")

	var issues []string
	for _, node := range nodes.Items {
		age := time.Since(node.CreationTimestamp.Time).Round(time.Second)

		var roles []string
		for label := range node.Labels {
			if role, found := strings.CutPrefix(label, "node-role.kubernetes.io/"); found && role != "" {
				roles = append(roles, role)
			}
		}
		if len(roles) == 0 {
			roles = []string{"<none>"}
		}
		slices.Sort(roles)

		status := "NotReady"
		var nodeProblems []string

		for _, cond := range node.Status.Conditions {
			switch cond.Type {
			case corev1.NodeReady:
				if cond.Status == corev1.ConditionTrue {
					status = "Ready"
				} else {
					status = string(cond.Status)
					nodeProblems = append(nodeProblems, fmt.Sprintf("Ready=%s", cond.Status))
					issues = append(issues, fmt.Sprintf("  - %s: Ready=%s (%s: %s)", node.Name, cond.Status, cond.Reason, cond.Message))
				}
			case corev1.NodeDiskPressure:
				if cond.Status == corev1.ConditionTrue {
					nodeProblems = append(nodeProblems, "DiskPressure")
					issues = append(issues, fmt.Sprintf("  - %s: DiskPressure=True (%s: %s)", node.Name, cond.Reason, cond.Message))
				}
			case corev1.NodeMemoryPressure:
				if cond.Status == corev1.ConditionTrue {
					nodeProblems = append(nodeProblems, "MemoryPressure")
					issues = append(issues, fmt.Sprintf("  - %s: MemoryPressure=True (%s: %s)", node.Name, cond.Reason, cond.Message))
				}
			case corev1.NodePIDPressure:
				if cond.Status == corev1.ConditionTrue {
					nodeProblems = append(nodeProblems, "PIDPressure")
					issues = append(issues, fmt.Sprintf("  - %s: PIDPressure=True (%s: %s)", node.Name, cond.Reason, cond.Message))
				}
			case corev1.NodeNetworkUnavailable:
				if cond.Status == corev1.ConditionTrue {
					nodeProblems = append(nodeProblems, "NetworkUnavailable")
					issues = append(issues, fmt.Sprintf("  - %s: NetworkUnavailable=True (%s: %s)", node.Name, cond.Reason, cond.Message))
				}
			}
		}

		for _, taint := range node.Spec.Taints {
			if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
				nodeProblems = append(nodeProblems, fmt.Sprintf("Taint:%s", taint.Key))
				issues = append(issues, fmt.Sprintf("  - %s: Scheduling taint %s=%s:%s", node.Name, taint.Key, taint.Value, taint.Effect))
			}
		}

		problemsStr := "None"
		if len(nodeProblems) > 0 {
			problemsStr = strings.Join(nodeProblems, ",")
		}

		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			node.Name,
			status,
			strings.Join(roles, ","),
			age,
			problemsStr,
		)
	}
	_ = writer.Flush()

	out := "Nodes:\n" + sb.String()
	if len(issues) > 0 {
		out += "\nNode Issues / Taints:\n" + strings.Join(issues, "\n")
	}
	return out, nil
}

// LogNodes logs a structured overview of node health, pressure conditions, and taints.
func LogNodes(clientset kubernetes.Interface, contextLabel string) {
	out, err := formatNodesStatuses(clientset)
	if err != nil {
		klog.V(2).Infof("Could not list nodes for %s: %v", contextLabel, err)
		return
	}
	klog.Info(out)
}

// getInvolvedObjectWorkloadKey returns a group key for event consolidation (e.g. "Pod/metrics-collector-deployment-bf4cc564f")
// and a wildcard name for display when multiple pods share the same event (e.g. "Pod/metrics-collector-deployment-bf4cc564f-*").
func getInvolvedObjectWorkloadKey(kind, name string) (groupKey, wildcardName string) {
	if kind == "Pod" && name != "" {
		if idx := strings.LastIndex(name, "-"); idx > 0 {
			prefix := name[:idx]
			return fmt.Sprintf("Pod/%s", prefix), fmt.Sprintf("Pod/%s-*", prefix)
		}
	}
	desc := kind
	if name != "" {
		desc = fmt.Sprintf("%s/%s", kind, name)
	}
	return desc, desc
}

// formatRecentWarningEvents queries and formats recent Warning events in the specified namespaces.
func formatRecentWarningEvents(clientset kubernetes.Interface, namespaces []string, window time.Duration, maxEvents int) (string, error) {
	if clientset == nil {
		return "", fmt.Errorf("clientset is nil")
	}

	type eventEntry struct {
		eventTime time.Time
		namespace string
		reason    string
		object    string
		message   string
		count     int32
	}

	eventMap := make(map[string]*eventEntry)
	now := time.Now()

	for _, ns := range namespaces {
		eventList, err := clientset.CoreV1().Events(ns).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			if !apierrors.IsNotFound(err) {
				klog.V(2).Infof("Could not list events in namespace %s: %v", ns, err)
			}
			continue
		}

		for _, e := range eventList.Items {
			if e.Type != corev1.EventTypeWarning {
				continue
			}

			var t time.Time
			switch {
			case !e.LastTimestamp.IsZero():
				t = e.LastTimestamp.Time
			case !e.EventTime.IsZero():
				t = e.EventTime.Time
			case !e.FirstTimestamp.IsZero():
				t = e.FirstTimestamp.Time
			case e.Series != nil && !e.Series.LastObservedTime.IsZero():
				t = e.Series.LastObservedTime.Time
			}

			if window > 0 && !t.IsZero() && now.Sub(t) > window {
				continue
			}

			groupKey, wildcardName := getInvolvedObjectWorkloadKey(e.InvolvedObject.Kind, e.InvolvedObject.Name)
			objDesc := e.InvolvedObject.Kind
			if e.InvolvedObject.Name != "" {
				objDesc = fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name)
			}

			count := e.Count
			if count == 0 && e.Series != nil && e.Series.Count > 0 {
				count = e.Series.Count
			}
			if count == 0 {
				count = 1
			}

			mapKey := fmt.Sprintf("%s|%s|%s|%s", ns, e.Reason, strings.TrimSpace(e.Message), groupKey)
			if existing, found := eventMap[mapKey]; found {
				existing.count += count
				if t.After(existing.eventTime) {
					existing.eventTime = t
				}
				existing.object = wildcardName
				continue
			}

			eventMap[mapKey] = &eventEntry{
				eventTime: t,
				namespace: ns,
				reason:    e.Reason,
				object:    objDesc,
				message:   e.Message,
				count:     count,
			}
		}
	}

	if len(eventMap) == 0 {
		return fmt.Sprintf("No recent Warning events found in namespaces: %s", strings.Join(namespaces, ", ")), nil
	}

	allEvents := make([]eventEntry, 0, len(eventMap))
	for _, entry := range eventMap {
		allEvents = append(allEvents, *entry)
	}

	slices.SortFunc(allEvents, func(a, b eventEntry) int {
		if a.eventTime.Equal(b.eventTime) {
			return 0
		}
		if a.eventTime.After(b.eventTime) {
			return -1
		}
		return 1
	})

	if maxEvents > 0 && len(allEvents) > maxEvents {
		allEvents = allEvents[:maxEvents]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Recent Warning Events (last %s, max %d):\n", window, maxEvents))
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "AGE\tNAMESPACE\tREASON\tOBJECT\tMESSAGE")

	for _, e := range allEvents {
		age := "-"
		if !e.eventTime.IsZero() {
			age = time.Since(e.eventTime).Round(time.Second).String()
		}

		reason := e.reason
		if e.count > 1 {
			reason = fmt.Sprintf("%s (x%d)", e.reason, e.count)
		}

		msg := truncateLogLine(strings.TrimSpace(e.message), 300)
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			age,
			e.namespace,
			reason,
			e.object,
			msg,
		)
	}
	_ = writer.Flush()
	return sb.String(), nil
}

func printRecentWarningEvents(clientset kubernetes.Interface, namespaces []string, window time.Duration, maxEvents int) {
	out, err := formatRecentWarningEvents(clientset, namespaces, window, maxEvents)
	if err != nil {
		klog.Errorf("Failed to format recent warning events: %v", err)
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

// isInternalServiceAccountSecret returns true if the secret is an internal OpenShift/Kubernetes
// service account dockercfg or token secret.
func isInternalServiceAccountSecret(secret corev1.Secret) bool {
	if secret.Type == corev1.SecretTypeDockercfg || secret.Type == corev1.SecretTypeServiceAccountToken {
		return true
	}
	if secret.Type == corev1.SecretTypeDockerConfigJson && strings.Contains(secret.Name, "-dockercfg-") {
		return true
	}
	return false
}

// formatSecretsStatuses formats a compact summary table of secrets in a namespace, omitting internal SA dockercfg/token secrets.
func formatSecretsStatuses(secrets []corev1.Secret, ns string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Secrets in namespace %s (total: %d):\n", ns, len(secrets)))
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tTYPE\tDATA\tAGE")

	saSecretCount := 0
	for _, secret := range secrets {
		if isInternalServiceAccountSecret(secret) {
			saSecretCount++
			continue
		}
		age := time.Since(secret.CreationTimestamp.Time).Round(time.Second)
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%d\t%s\n",
			secret.Name,
			secret.Type,
			len(secret.Data),
			age)
	}
	_ = writer.Flush()
	if saSecretCount > 0 {
		sb.WriteString(fmt.Sprintf("  (+ %d service account dockercfg/token Secrets omitted for brevity)\n", saSecretCount))
	}
	return sb.String()
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

	klog.Info(formatSecretsStatuses(secrets.Items, ns))
}

// formatAddOnDeploymentConfig generates a concise, human-readable summary of an AddOnDeploymentConfig.
func formatAddOnDeploymentConfig(obj *unstructured.Unstructured) string {
	var sb strings.Builder
	name := obj.GetName()
	gen := obj.GetGeneration()
	sb.WriteString(fmt.Sprintf("  - %s (generation %d):\n", name, gen))

	installNS, found, _ := unstructured.NestedString(obj.Object, "spec", "agentInstallNamespace")
	if found && installNS != "" {
		sb.WriteString(fmt.Sprintf("      agentInstallNamespace: %s\n", installNS))
	}

	customVars, found, _ := unstructured.NestedSlice(obj.Object, "spec", "customizedVariables")
	if found && len(customVars) > 0 {
		sb.WriteString("      customizedVariables:\n")
		for _, cv := range customVars {
			if cvMap, ok := cv.(map[string]any); ok {
				varName := cvMap["name"]
				varVal := cvMap["value"]
				sb.WriteString(fmt.Sprintf("        %v: %v\n", varName, varVal))
			}
		}
	}

	proxyConfig, found, _ := unstructured.NestedMap(obj.Object, "spec", "proxyConfig")
	if found && len(proxyConfig) > 0 {
		sb.WriteString("      proxyConfig:\n")
		for _, k := range slices.Sorted(maps.Keys(proxyConfig)) {
			if strVal, ok := proxyConfig[k].(string); ok && strVal != "" {
				sb.WriteString(fmt.Sprintf("        %s: %s\n", k, strVal))
			}
		}
	}

	return sb.String()
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
		adcInfo.WriteString(formatAddOnDeploymentConfig(&obj))
	}
	klog.Info(adcInfo.String())
}

func sanitizeManifestError(msg string) string {
	return truncateLogLine(strings.TrimSpace(msg), 600)
}

func formatManifestWorks(items []unstructured.Unstructured) string {
	var sb strings.Builder
	sb.WriteString("Observability ManifestWorks:\n")
	writer := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAMESPACE\tNAME\tAPPLIED\tAVAILABLE\tDEGRADED\tDELETING\tFINALIZERS\tMANIFESTS")

	type degradedMW struct {
		ns     string
		name   string
		detail string
	}
	var degraded []degradedMW
	count := 0

	for _, obj := range items {
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
		deleting := statusNo
		if obj.GetDeletionTimestamp() != nil {
			deleting = statusYes
		}
		finalizers := strings.Join(obj.GetFinalizers(), ",")

		applied := statusUnknown
		available := statusUnknown
		degradedCond := statusUnknown

		if deleting == statusYes {
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

		var manifestKinds []string
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
				if kind != "" && !slices.Contains(manifestKinds, kind) {
					manifestKinds = append(manifestKinds, kind)
				}

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
								detail: fmt.Sprintf("Manifest %s %s/%s [%s=%s (%s): %s]", kind, resNs, resName, mType, mStatus, mReason, sanitizeManifestError(mMsg)),
							})
						}
					}
				}
			}
		}

		manifestsSummary := "-"
		if len(manifestKinds) > 0 {
			manifestsSummary = fmt.Sprintf("[%s]", strings.Join(manifestKinds, ","))
		}

		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t[%s]\t%s\n",
			ns,
			name,
			applied,
			available,
			degradedCond,
			deleting,
			finalizers,
			manifestsSummary,
		)
	}
	_ = writer.Flush()

	if count == 0 {
		return ""
	}

	if len(degraded) > 0 {
		sb.WriteString("\nDegraded or Terminating ManifestWorks Details:\n")
		for _, d := range degraded {
			detail := truncateLogLine(d.detail, 700)
			sb.WriteString(fmt.Sprintf("  - %s/%s: %s\n", d.ns, d.name, detail))
		}
	}

	return sb.String()
}

func printManifestWorks(client dynamic.Interface) {
	gvr := NewOCMManifestworksGVR()
	objs, err := client.Resource(gvr).Namespace("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("failed to list ManifestWorks: %v", err)
		return
	}

	out := formatManifestWorks(objs.Items)
	if out == "" {
		klog.V(1).Info("No observability ManifestWorks found")
		return
	}
	klog.Info(out)
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

// printMCOACustomResources lists PrometheusAgent and ScrapeConfig custom resources in the namespace.
func printMCOACustomResources(client dynamic.Interface, ns string) {
	if client == nil {
		return
	}
	paGVR := NewPrometheusAgentGVR()
	paList, err := client.Resource(paGVR).Namespace(ns).List(context.TODO(), metav1.ListOptions{})
	if err == nil && len(paList.Items) > 0 {
		names := make([]string, 0, len(paList.Items))
		for _, item := range paList.Items {
			names = append(names, item.GetName())
		}
		klog.Infof("PrometheusAgents in %s (%d): %s", ns, len(names), strings.Join(names, ", "))
	}

	scGVR := NewScrapeConfigGVR()
	scList, err := client.Resource(scGVR).Namespace(ns).List(context.TODO(), metav1.ListOptions{})
	if err == nil && len(scList.Items) > 0 {
		names := make([]string, 0, len(scList.Items))
		for _, item := range scList.Items {
			names = append(names, item.GetName())
		}
		klog.Infof("ScrapeConfigs in %s (%d): %s", ns, len(names), strings.Join(names, ", "))
	}

	prGVR := NewPrometheusRuleGVR()
	prList, err := client.Resource(prGVR).Namespace(ns).List(context.TODO(), metav1.ListOptions{})
	if err == nil && len(prList.Items) > 0 {
		names := make([]string, 0, len(prList.Items))
		for _, item := range prList.Items {
			names = append(names, item.GetName())
		}
		klog.Infof("PrometheusRules in %s (%d): %s", ns, len(names), strings.Join(names, ", "))
	}
}

// logClusterMonitoringConfigStatus summarizes the state of OpenShift cluster monitoring
// ConfigMaps (CMO / UWM) which control Prometheus in-cluster alert forwarding.
func logClusterMonitoringConfigStatus(client kubernetes.Interface, clusterLabel string) {
	if client == nil {
		return
	}
	cmTargets := []struct {
		ns   string
		name string
	}{
		{ns: "openshift-monitoring", name: "cluster-monitoring-config"},
		{ns: "openshift-user-workload-monitoring", name: "user-workload-monitoring-config"},
	}

	for _, target := range cmTargets {
		cm, err := client.CoreV1().ConfigMaps(target.ns).Get(context.TODO(), target.name, metav1.GetOptions{})
		if err != nil {
			if !apierrors.IsNotFound(err) {
				klog.V(2).Infof("Could not get ConfigMap %s/%s on %s: %v", target.ns, target.name, clusterLabel, err)
			}
			continue
		}

		configData, ok := cm.Data["config.yaml"]
		if !ok || strings.TrimSpace(configData) == "" {
			klog.Infof("ConfigMap %s/%s on %s: exists (empty config.yaml)", target.ns, target.name, clusterLabel)
			continue
		}

		hasAlertmanager := strings.Contains(configData, "additionalAlertmanagerConfigs:")
		hasUWM := strings.Contains(configData, "enableUserWorkload: true")

		var flags []string
		if hasAlertmanager {
			flags = append(flags, "additionalAlertmanagerConfigs=present")
		} else {
			flags = append(flags, "additionalAlertmanagerConfigs=absent")
		}
		if target.ns == "openshift-monitoring" {
			if hasUWM {
				flags = append(flags, "enableUserWorkload=true")
			} else {
				flags = append(flags, "enableUserWorkload=false/unspecified")
			}
		}

		klog.Infof("ConfigMap %s/%s on %s: %s", target.ns, target.name, clusterLabel, strings.Join(flags, ", "))
	}
}

// logSpokeClusterDebugInfo runs diagnostics on the spoke cluster's observability workloads.
func logSpokeClusterDebugInfo(
	spokeClient kubernetes.Interface,
	spokeDynClient dynamic.Interface,
	clusterName string,
	isMCOA bool,
) {
	if spokeClient == nil || spokeDynClient == nil {
		return
	}
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
		LogNodes(spokeClient, clusterName)
		CheckDeploymentsInNamespace(spokeClient, MCO_AGENT_ADDON_NAMESPACE)
		CheckStatefulSetsInNamespace(spokeClient, MCO_AGENT_ADDON_NAMESPACE)
		CheckJobsInNamespace(spokeClient, MCO_AGENT_ADDON_NAMESPACE)
		CheckPodsInNamespace(spokeClient, MCO_AGENT_ADDON_NAMESPACE, []string{"endpoint-monitoring-operator", "prom-agent", "observability-monitoring-cleanup"}, map[string]string{})
		printMCOACustomResources(spokeDynClient, MCO_AGENT_ADDON_NAMESPACE)
		logClusterMonitoringConfigStatus(spokeClient, clusterName)
		printRecentWarningEvents(spokeClient, []string{MCO_AGENT_ADDON_NAMESPACE}, 20*time.Minute, 15)
	} else {
		klog.Infof("%s (Legacy: %s)", SectionSpokeWorkloads, clusterName)
		LogNodes(spokeClient, clusterName)
		PrintObject(context.TODO(), spokeDynClient, NewMCOAddonGVR(), MCO_ADDON_NAMESPACE, "observability-addon")
		CheckDeploymentsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		CheckStatefulSetsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		CheckDaemonSetsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		CheckJobsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		CheckPodsInNamespace(spokeClient, MCO_ADDON_NAMESPACE, []string{"observability-addon"}, map[string]string{})
		printConfigMapsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		printSecretsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		logClusterMonitoringConfigStatus(spokeClient, clusterName)
		printRecentWarningEvents(spokeClient, []string{MCO_ADDON_NAMESPACE}, 20*time.Minute, 15)
	}
}
