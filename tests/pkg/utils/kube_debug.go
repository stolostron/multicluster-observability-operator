// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// LogFailingTestStandardDebugInfo logs standard debug info for failing tests.
// It scans workloads and pods from hub and managed clusters observability namespaces.
// It also prints MCO and OBA objects.
// If a workload or pod is not running, it prints the resource spec, status, events and logs if appropriate.
func LogFailingTestStandardDebugInfo(opt TestOptions) {
	klog.V(1).Infof("Test failed, printing debug info. TestOptions: %+v", opt)

	// Print MCO object
	hubDynClient := NewKubeClientDynamic(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	PrintObject(context.TODO(), hubDynClient, NewMCOGVRV1BETA2(), "", MCO_CR_NAME)

	// Check pods in hub
	hubClient := NewKubeClient(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
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
	LogManagedClusters(hubDynClient)

	CheckDeploymentsInNamespace(hubClient, MCO_AGENT_ADDON_NAMESPACE)
	CheckStatefulSetsInNamespace(hubClient, MCO_AGENT_ADDON_NAMESPACE)
	CheckPodsInNamespace(hubClient, MCO_AGENT_ADDON_NAMESPACE, []string{}, map[string]string{})

	for _, mc := range opt.ManagedClusters {
		if mc.Name == "local-cluster" {
			// Skip local-cluster as same namespace as hub, and already checked
			continue
		}

		spokeDynClient := NewKubeClientDynamic(mc.ClusterServerURL, mc.KubeConfig, mc.KubeContext)
		PrintObject(context.TODO(), spokeDynClient, NewMCOAddonGVR(), MCO_ADDON_NAMESPACE, "observability-addon")

		spokeClient := NewKubeClient(mc.ClusterServerURL, mc.KubeConfig, mc.KubeContext)
		CheckDeploymentsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		CheckStatefulSetsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		CheckDaemonSetsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		CheckPodsInNamespace(spokeClient, MCO_ADDON_NAMESPACE, []string{"observability-addon"}, map[string]string{})
		printConfigMapsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		printSecretsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
	}
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
		klog.V(1).Infof("No pods in namespace %s", ns)
	}

	klog.V(1).Infof("Checking %d pods in namespace %q", len(pods.Items), ns)
	printPodsStatuses(pods.Items)

	notRunningPodsCount := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
			notRunningPodsCount++
		}

		force := false
		for _, forcePodName := range forcePodNamesLog {
			if strings.Contains(pod.Name, forcePodName) {
				force = true
				break
			}
		}
		if (pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodSucceeded) && !force {
			continue
		}

		// print pod spec
		podSpec, err := json.MarshalIndent(pod.Spec, "", "  ")
		if err != nil {
			klog.Errorf("Failed to marshal pod %q spec: %s", pod.Name, err.Error())
		}
		klog.V(1).Infof("Pod %q spec: \n%s", pod.Name, string(podSpec))

		LogPodStatus(pod)
		LogObjectEvents(client, ns, "Pod", pod.Name)
		LogPodLogs(client, ns, pod)
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

func LogPodLogs(client kubernetes.Interface, ns string, pod corev1.Pod) {
	for _, container := range pod.Spec.Containers {
		logsRes := client.CoreV1().Pods(ns).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container: container.Name,
		}).Do(context.Background())

		if logsRes.Error() != nil {
			klog.Errorf("Failed to get logs for pod %q: %s", pod.Name, logsRes.Error())
			continue
		}

		logs, err := logsRes.Raw()
		if err != nil {
			klog.Errorf("Failed to get logs for pod %q container %q: %s", pod.Name, container.Name, err.Error())
			continue
		}

		// Aggregate info, debug and error logs from the past 6 minutes
		filteredLines := []string{}
		lines := strings.Split(string(logs), "\n")
		cutoffTime := time.Now().Add(-6 * time.Minute)

		for i := len(lines) - 1; i >= 0; i-- {
			line := lines[i]
			// Try to parse timestamp at the beginning of the line
			// Format: 2025-12-17T18:56:14.441Z
			fields := strings.Fields(line)
			if len(fields) > 0 {
				if t, err := time.Parse(time.RFC3339, fields[0]); err == nil {
					if t.Before(cutoffTime) {
						break
					}
				}
			}

			lowerLine := strings.ToLower(line)
			if strings.Contains(lowerLine, "error") || strings.Contains(lowerLine, "info") || strings.Contains(lowerLine, "debug") {
				filteredLines = append(filteredLines, line)
			}
		}

		// Reverse the lines to restore order
		for i, j := 0, len(filteredLines)-1; i < j; i, j = i+1, j-1 {
			filteredLines[i], filteredLines[j] = filteredLines[j], filteredLines[i]
		}

		logs = []byte(strings.Join(filteredLines, "\n"))

		delimitedLogs := fmt.Sprintf(">>>>>>>>>> container logs >>>>>>>>>>\n%s<<<<<<<<<< container logs <<<<<<<<<<", string(logs))
		klog.V(1).Infof("Pod %q container %q logs (aggregated info/debug/error from the last 6 minutes): \n%s", pod.Name, container.Name, delimitedLogs)
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
	}

	klog.V(1).Infof("Deployments in namespace %s: \n", ns)
	printDeploymentsStatuses(client, ns)

	for _, deployment := range deployments.Items {
		if deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas {
			continue
		}

		// print deployment spec
		deploymentSpec, err := json.MarshalIndent(deployment.Spec, "", "  ")
		if err != nil {
			klog.Errorf("Failed to marshal deployment %q spec: %s", deployment.Name, err.Error())
		}
		klog.V(1).Infof("Deployment %q spec: \n%s", deployment.Name, string(deploymentSpec))

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

	klog.V(1).Infof("StatefulSets in namespace %s: \n", ns)
	printStatefulSetsStatuses(client, ns)

	for _, statefulSet := range statefulSets.Items {
		if statefulSet.Status.UpdatedReplicas == *statefulSet.Spec.Replicas {
			continue
		}

		// Print statefulset spec
		statefulSetSpec, err := json.MarshalIndent(statefulSet.Spec, "", "  ")
		if err != nil {
			klog.Errorf("Failed to marshal statefulset %q spec: %s", statefulSet.Name, err.Error())
		}
		klog.V(1).Infof("StatefulSet %q spec: \n%s", statefulSet.Name, string(statefulSetSpec))

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

	klog.V(1).Infof("DaemonSets in namespace %s: \n", ns)
	printDaemonSetsStatuses(client, ns)

	for _, daemonSet := range daemonSets.Items {
		if daemonSet.Status.UpdatedNumberScheduled == daemonSet.Status.DesiredNumberScheduled {
			continue
		}

		// Print daemonset spec
		daemonSetSpec, err := json.MarshalIndent(daemonSet.Spec, "", "  ")
		if err != nil {
			klog.Errorf("Failed to marshal daemonset %q spec: %s", daemonSet.Name, err.Error())
		}
		klog.V(1).Infof("DaemonSet %q spec: \n%s", daemonSet.Name, string(daemonSetSpec))

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
		klog.Errorf("failed to get ManagedClusters: %v", err)
		return
	}

	var managedClustersData strings.Builder
	managedClustersData.WriteString(">>>>>>>>>> Managed Clusters >>>>>>>>>>\n")
	for _, obj := range objs.Items {
		managedCluster := &clusterv1.ManagedCluster{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, managedCluster)
		if err != nil {
			klog.Errorf("failed to convert unstructured to ManagedCluster %s: %v", obj.GetName(), err)
			continue
		}

		managedCluster.ManagedFields = nil

		for i := range managedCluster.Spec.ManagedClusterClientConfigs {
			if len(managedCluster.Spec.ManagedClusterClientConfigs[i].CABundle) > 0 {
				managedCluster.Spec.ManagedClusterClientConfigs[i].CABundle = []byte("xxx_omitted")
			}
		}

		mc, err := json.MarshalIndent(managedCluster, "", "  ")
		if err != nil {
			klog.Errorf("Failed to marshal ManagedCluster %q: %s", managedCluster.GetName(), err.Error())
		}
		managedClustersData.WriteString(string(mc) + "\n")
	}
	managedClustersData.WriteString("<<<<<<<<<< Managed Clusters <<<<<<<<<<")
	klog.Info(managedClustersData.String())
}

func printPodsStatuses(pods []corev1.Pod) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(writer, "NAME\tSTATUS\tRESTARTS\tAGE")
	for _, pod := range pods {
		var restartCount int32
		if len(pod.Status.ContainerStatuses) > 0 {
			restartCount = pod.Status.ContainerStatuses[0].RestartCount
		}
		age := time.Since(pod.CreationTimestamp.Time).Round(time.Second)
		fmt.Fprintf(writer, "%s\t%s\t%d\t%s\n",
			pod.Name,
			pod.Status.Phase,
			restartCount,
			age)
	}
	writer.Flush()
}

func printDeploymentsStatuses(clientset kubernetes.Interface, namespace string) {
	deploymentsClient := clientset.AppsV1().Deployments(namespace)
	deployments, err := deploymentsClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(writer, "NAME\tREADY\tUP-TO-DATE\tAVAILABLE\tAGE")
	for _, deployment := range deployments.Items {
		ready := fmt.Sprintf("%d/%d", deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
		age := time.Since(deployment.CreationTimestamp.Time).Round(time.Second)
		fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%s\n",
			deployment.Name,
			ready,
			deployment.Status.UpdatedReplicas,
			deployment.Status.AvailableReplicas,
			age)
	}
	writer.Flush()
}

func printStatefulSetsStatuses(clientset kubernetes.Interface, namespace string) {
	statefulSetsClient := clientset.AppsV1().StatefulSets(namespace)
	statefulSets, err := statefulSetsClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(writer, "NAME\tREADY\tAGE")
	for _, statefulSet := range statefulSets.Items {
		ready := fmt.Sprintf("%d/%d", statefulSet.Status.ReadyReplicas, *statefulSet.Spec.Replicas)
		age := time.Since(statefulSet.CreationTimestamp.Time).Round(time.Second)
		fmt.Fprintf(writer, "%s\t%s\t%s\n",
			statefulSet.Name,
			ready,
			age)
	}
	writer.Flush()
}

func printDaemonSetsStatuses(clientset kubernetes.Interface, namespace string) {
	daemonSetsClient := clientset.AppsV1().DaemonSets(namespace)
	daemonSets, err := daemonSetsClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(writer, "NAME\tDESIRED\tCURRENT\tREADY\tAGE")
	for _, daemonSet := range daemonSets.Items {
		age := time.Since(daemonSet.CreationTimestamp.Time).Round(time.Second)
		fmt.Fprintf(writer, "%s\t%d\t%d\t%d\t%s\n",
			daemonSet.Name,
			daemonSet.Status.DesiredNumberScheduled,
			daemonSet.Status.CurrentNumberScheduled,
			daemonSet.Status.NumberReady,
			age)
	}
	writer.Flush()
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

	klog.V(1).Infof("ConfigMaps in namespace %s: \n", ns)
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(writer, "NAME\tDATA\tAGE")
	for _, configMap := range configMaps.Items {
		age := time.Since(configMap.CreationTimestamp.Time).Round(time.Second)
		fmt.Fprintf(writer, "%s\t%d\t%s\n",
			configMap.Name,
			len(configMap.Data),
			age)
	}
	writer.Flush()
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

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(writer, "NAME\tTYPE\tDATA\tAGE")
	for _, secret := range secrets.Items {
		age := time.Since(secret.CreationTimestamp.Time).Round(time.Second)
		fmt.Fprintf(writer, "%s\t%s\t%d\t%s\n",
			secret.Name,
			secret.Type,
			len(secret.Data),
			age)
	}
	writer.Flush()
}
