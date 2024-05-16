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
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
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
	PrintObject(context.TODO(), hubDynClient, NewMCOGVRV1BETA2(), MCO_NAMESPACE, MCO_CR_NAME)

	// Check pods in hub
	hubClient := NewKubeClient(
		opt.HubCluster.ClusterServerURL,
		opt.KubeConfig,
		opt.HubCluster.KubeContext)
	LogMCOOperatorDebugInfo(hubClient)
	CheckDeploymentsInNamespace(hubClient, MCO_NAMESPACE)
	CheckPodsInNamespace(hubClient, MCO_NAMESPACE)

	for _, mc := range opt.ManagedClusters {
		if mc.Name == "local-cluster" {
			continue
		}

		spokeDynClient := NewKubeClientDynamic(mc.ClusterServerURL, opt.KubeConfig, mc.KubeContext)
		PrintObject(context.TODO(), spokeDynClient, NewMCOAddonGVR(), MCO_ADDON_NAMESPACE, "observability-addon")

		spokeClient := NewKubeClient(mc.ClusterServerURL, mc.KubeConfig, mc.KubeContext)
		CheckDeploymentsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
		CheckPodsInNamespace(spokeClient, MCO_ADDON_NAMESPACE)
	}
}

// CheckPodsInNamespace lists pods in a namespace and logs debug info (status, events, logs) for pods not running.
func CheckPodsInNamespace(client kubernetes.Interface, ns string) {
	pods, err := client.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
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
		if pod.Status.Phase != corev1.PodRunning {
			notRunningPodsCount++
		}

		// Alway log details of the addon pod
		if pod.Status.Phase == corev1.PodRunning && pod.Name != "observability-addon" {
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

func LogMCOOperatorDebugInfo(client kubernetes.Interface) {
	// get podList with label name: multicluster-observability-operator
	podList, err := client.CoreV1().Pods("open-cluster-management").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "name=multicluster-observability-operator",
	})
	if err != nil {
		klog.Errorf("Failed to get pod with label name=multicluster-observability-operator in namespace open-cluster-management: %v", err)
		return
	}

	if len(podList.Items) == 0 {
		klog.V(1).Infof("No pods with label name=multicluster-observability-operator in namespace open-cluster-management")
	}

	if len(podList.Items) > 1 {
		klog.Errorf("Found more than one pod with label name=multicluster-observability-operator in namespace open-cluster-management")
	}

	pod := podList.Items[0]
	klog.V(1).Infof("Logging debug info for MCO operator pod %q in namespace %q", pod.Name, pod.Namespace)
	LogPodLogs(client, "open-cluster-management", pod)
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

		// Filter error logs and keep all last 150 lines
		cleanedLines := []string{}
		lines := strings.Split(string(logs), "\n")
		for i, line := range lines {
			if strings.Contains(strings.ToLower(line), "error") || i > len(lines)-150 {
				cleanedLines = append(cleanedLines, line)
			}
		}

		logs = []byte(strings.Join(cleanedLines, "\n"))

		delimitedLogs := fmt.Sprintf(">>>>>>>>>> container logs >>>>>>>>>>\n%s<<<<<<<<<< container logs <<<<<<<<<<", string(logs))
		klog.V(1).Infof("Pod %q container %q logs: \n%s", pod.Name, container.Name, delimitedLogs)
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

func printStatefulSetsStatuses(clientset *kubernetes.Clientset, namespace string) {
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

func printDaemonSetsStatuses(clientset *kubernetes.Clientset, namespace string) {
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
