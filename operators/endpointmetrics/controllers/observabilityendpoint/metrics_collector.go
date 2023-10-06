// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package observabilityendpoint

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/rendering"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
)

const (
	metricsConfigMapKey       = "metrics_list.yaml"
	metricsOcp311ConfigMapKey = "ocp311_metrics_list.yaml"
	metricsCollectorName      = "metrics-collector-deployment"
	selectorKey               = "component"
	selectorValue             = "metrics-collector"
	caMounthPath              = "/etc/serving-certs-ca-bundle"
	caVolName                 = "serving-certs-ca-bundle"
	mtlsCertName              = "observability-controller-open-cluster-management.io-observability-signer-client-cert"
	mtlsCaName                = "observability-managed-cluster-certs"
	limitBytes                = 1073741824
	defaultInterval           = "30s"
)

const (
	restartLabel = "cert/time-restarted"
)

var (
	ocpPromURL = "https://prometheus-k8s.openshift-monitoring.svc:9091"
	promURL    = "https://prometheus-k8s:9091"
)

func createDeployment(clusterID string, clusterType string,
	obsAddonSpec oashared.ObservabilityAddonSpec,
	hubInfo operatorconfig.HubInfo, allowlist operatorconfig.MetricsAllowlist,
	nodeSelector map[string]string, tolerations []corev1.Toleration,
	replicaCount int32) *appsv1.Deployment {
	interval := fmt.Sprint(obsAddonSpec.Interval) + "s"
	if fmt.Sprint(obsAddonSpec.Interval) == "" {
		interval = defaultInterval
	}
	evaluateInterval := "30s"
	if obsAddonSpec.Interval < 30 {
		evaluateInterval = interval
	}

	volumes := []corev1.Volume{
		{
			Name: "mtlscerts",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: mtlsCertName,
				},
			},
		},
		{
			Name: "mtlsca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: mtlsCaName,
				},
			},
		},
	}
	mounts := []corev1.VolumeMount{
		{
			Name:      "mtlscerts",
			MountPath: "/tlscerts/certs",
		},
		{
			Name:      "mtlsca",
			MountPath: "/tlscerts/ca",
		},
	}
	caFile := caMounthPath + "/service-ca.crt"
	if clusterID == "" {
		clusterID = hubInfo.ClusterName
		// deprecated ca bundle, only used for ocp 3.11 env
		caFile = "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
	} else {
		volumes = append(volumes, corev1.Volume{
			Name: caVolName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: caConfigmapName,
					},
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      caVolName,
			MountPath: caMounthPath,
		})
	}

	commands := []string{
		"/usr/bin/metrics-collector",
		"--from=$(FROM)",
		"--to-upload=$(TO)",
		"--to-upload-ca=/tlscerts/ca/ca.crt",
		"--to-upload-cert=/tlscerts/certs/tls.crt",
		"--to-upload-key=/tlscerts/certs/tls.key",
		"--interval=" + interval,
		"--evaluate-interval=" + evaluateInterval,
		"--limit-bytes=" + strconv.Itoa(limitBytes),
		fmt.Sprintf("--label=\"cluster=%s\"", hubInfo.ClusterName),
		fmt.Sprintf("--label=\"clusterID=%s\"", clusterID),
	}
	commands = append(commands, "--from-token-file=/var/run/secrets/kubernetes.io/serviceaccount/token")
	if !installPrometheus {
		commands = append(commands, "--from-ca-file="+caFile)
	}
	if clusterType != "" {
		commands = append(commands, fmt.Sprintf("--label=\"clusterType=%s\"", clusterType))
	}

	dynamicMetricList := map[string]bool{}
	for _, group := range allowlist.CollectRuleGroupList {
		if group.Selector.MatchExpression != nil {
			for _, expr := range group.Selector.MatchExpression {
				if !evluateMatchExpression(expr, clusterID, clusterType, obsAddonSpec, hubInfo,
					allowlist, nodeSelector, tolerations, replicaCount) {
					continue
				}
				for _, rule := range group.CollectRuleList {
					matchList := []string{}
					for _, match := range rule.Metrics.MatchList {
						matchList = append(matchList, `"`+strings.ReplaceAll(match, `"`, `\"`)+`"`)
						if name := getNameInMatch(match); name != "" {
							dynamicMetricList[name] = false
						}
					}
					for _, name := range rule.Metrics.NameList {
						dynamicMetricList[name] = false
					}
					matchListStr := "[" + strings.Join(matchList, ",") + "]"
					nameListStr := `["` + strings.Join(rule.Metrics.NameList, `","`) + `"]`
					commands = append(
						commands,
						fmt.Sprintf("--collectrule={\"name\":\"%s\",\"expr\":\"%s\",\"for\":\"%s\",\"names\":%v,\"matches\":%v}",
							rule.Collect, rule.Expr, rule.For, nameListStr, matchListStr),
					)
				}
			}
		}
	}

	for _, metrics := range allowlist.NameList {
		if _, ok := dynamicMetricList[metrics]; !ok {
			commands = append(commands, fmt.Sprintf("--match={__name__=\"%s\"}", metrics))
		}
	}
	for _, match := range allowlist.MatchList {
		if name := getNameInMatch(match); name != "" {
			if _, ok := dynamicMetricList[name]; ok {
				continue
			}
		}
		commands = append(commands, fmt.Sprintf("--match={%s}", match))
	}

	renamekeys := make([]string, 0, len(allowlist.RenameMap))
	for k := range allowlist.RenameMap {
		renamekeys = append(renamekeys, k)
	}
	sort.Strings(renamekeys)
	for _, k := range renamekeys {
		commands = append(commands, fmt.Sprintf("--rename=\"%s=%s\"", k, allowlist.RenameMap[k]))
	}
	for _, rule := range allowlist.RecordingRuleList {
		commands = append(
			commands,
			fmt.Sprintf("--recordingrule={\"name\":\"%s\",\"query\":\"%s\"}", rule.Record, rule.Expr),
		)
	}

	from := promURL
	if !installPrometheus {
		from = ocpPromURL
	}
	metricsCollectorDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      metricsCollectorName,
			Namespace: namespace,
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(replicaCount),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					selectorKey: selectorValue,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ownerLabelKey: ownerLabelValue,
						operatorconfig.WorkloadPartitioningPodAnnotationKey: operatorconfig.WorkloadPodExpectedValueJSON,
					},
					Labels: map[string]string{
						selectorKey: selectorValue,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName,
					Containers: []corev1.Container{
						{
							Name:    "metrics-collector",
							Image:   rendering.Images[operatorconfig.MetricsCollectorKey],
							Command: commands,
							Env: []corev1.EnvVar{
								{
									Name:  "FROM",
									Value: from,
								},
								{
									Name:  "TO",
									Value: hubInfo.ObservatoriumAPIEndpoint,
								},
							},
							VolumeMounts:    mounts,
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
					Volumes:      volumes,
					NodeSelector: nodeSelector,
					Tolerations:  tolerations,
				},
			},
		},
	}
	if obsAddonSpec.Resources != nil {
		metricsCollectorDep.Spec.Template.Spec.Containers[0].Resources = *obsAddonSpec.Resources
	}
	return metricsCollectorDep
}

func updateMetricsCollector(ctx context.Context, client client.Client, obsAddonSpec oashared.ObservabilityAddonSpec,
	hubInfo operatorconfig.HubInfo, clusterID string, clusterType string,
	replicaCount int32, forceRestart bool) (bool, error) {

	list, err := getMetricsAllowlist(ctx, client, clusterType)
	if err != nil {
		return false, err
	}
	endpointDeployment := getEndpointDeployment(ctx, client)
	log.Info(fmt.Sprintf("endpoint operator [version: %+v, UID: %+v]",
		endpointDeployment.ResourceVersion,
		endpointDeployment.UID))
	log.Info(fmt.Sprintf("node selectors: %+v", endpointDeployment.Spec.Template.Spec.NodeSelector))
	log.Info(fmt.Sprintf("tolerations: %+v", endpointDeployment.Spec.Template.Spec.Tolerations))
	deployment := createDeployment(
		clusterID,
		clusterType,
		obsAddonSpec,
		hubInfo,
		list,
		endpointDeployment.Spec.Template.Spec.NodeSelector,
		endpointDeployment.Spec.Template.Spec.Tolerations,
		replicaCount,
	)
	found := &appsv1.Deployment{}
	err = client.Get(ctx, types.NamespacedName{Name: metricsCollectorName,
		Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			err = client.Create(ctx, deployment)
			if err != nil {
				log.Error(err, "Failed to create metrics-collector deployment")
				return false, err
			}
			log.Info("Created metrics-collector deployment ")
		} else {
			log.Error(err, "Failed to check the metrics-collector deployment")
			return false, err
		}
	} else {
		if !reflect.DeepEqual(deployment.Spec.Template.Spec, found.Spec.Template.Spec) ||
			!reflect.DeepEqual(deployment.Spec.Replicas, found.Spec.Replicas) ||
			forceRestart {
			deployment.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
			if forceRestart && found.Status.ReadyReplicas != 0 {
				deployment.Spec.Template.ObjectMeta.Labels[restartLabel] = time.Now().Format("2006-1-2.1504")
			}
			err = client.Update(ctx, deployment)
			if err != nil {
				log.Error(err, "Failed to update metrics-collector deployment")
				return false, err
			}
			log.Info("Updated metrics-collector deployment ")
		}
	}
	return true, nil
}

func deleteMetricsCollector(ctx context.Context, client client.Client) error {
	found := &appsv1.Deployment{}
	err := client.Get(ctx, types.NamespacedName{Name: metricsCollectorName,
		Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("The metrics collector deployment does not exist")
			return nil
		}
		log.Error(err, "Failed to check the metrics collector deployment")
		return err
	}
	err = client.Delete(ctx, found)
	if err != nil {
		log.Error(err, "Failed to delete the metrics collector deployment")
		return err
	}
	log.Info("metrics collector deployment deleted")
	return nil
}

func int32Ptr(i int32) *int32 { return &i }

func getMetricsAllowlist(ctx context.Context, client client.Client,
	clusterType string) (operatorconfig.MetricsAllowlist, error) {
	l := &operatorconfig.MetricsAllowlist{}
	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Name: operatorconfig.AllowlistConfigMapName,
		Namespace: namespace}, cm)
	if err != nil {
		log.Error(err, "Failed to get configmap")
	} else {
		if cm.Data != nil {
			configmapKey := metricsConfigMapKey
			if clusterType == "ocp3" {
				configmapKey = metricsOcp311ConfigMapKey
			}
			err = yaml.Unmarshal([]byte(cm.Data[configmapKey]), l)
			if err != nil {
				log.Error(err, "Failed to unmarshal data in configmap")
				return *l, err
			}
		}
	}
	return *l, nil
}

func getEndpointDeployment(ctx context.Context, client client.Client) appsv1.Deployment {
	d := &appsv1.Deployment{}
	err := client.Get(ctx, types.NamespacedName{Name: "endpoint-observability-operator", Namespace: namespace}, d)
	if err != nil {
		log.Error(err, "Failed to get deployment")
	}
	return *d
}

func getNameInMatch(match string) string {
	r := regexp.MustCompile(`__name__="([^,]*)"`)
	m := r.FindAllStringSubmatch(match, -1)
	if m != nil {
		return m[0][1]
	}
	return ""
}
