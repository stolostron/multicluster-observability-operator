// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package observabilityendpoint

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/open-cluster-management/multicluster-observability-operator/operators/endpointmetrics/pkg/rendering"
	oashared "github.com/open-cluster-management/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	operatorconfig "github.com/open-cluster-management/multicluster-observability-operator/operators/pkg/config"
)

const (
	metricsConfigMapKey  = "metrics_list.yaml"
	metricsCollectorName = "metrics-collector-deployment"
	selectorKey          = "component"
	selectorValue        = "metrics-collector"
	caMounthPath         = "/etc/serving-certs-ca-bundle"
	caVolName            = "serving-certs-ca-bundle"
	mtlsCertName         = "observability-controller-open-cluster-management.io-observability-signer-client-cert"
	mtlsCaName           = "observability-managed-cluster-certs"
	limitBytes           = 1073741824
	defaultInterval      = "30s"
)

const (
	restartLabel = "cert/time-restarted"
)

var (
	ocpPromURL = "https://prometheus-k8s.openshift-monitoring.svc:9091"
	promURL    = "https://prometheus-k8s:9091"
)

type MetricsAllowlist struct {
	NameList  []string          `yaml:"names"`
	MatchList []string          `yaml:"matches"`
	ReNameMap map[string]string `yaml:"renames"`
	RuleList  []Rule            `yaml:"rules"`
}

// Rule is the struct for recording rules and alert rules
type Rule struct {
	Record string `yaml:"record"`
	Expr   string `yaml:"expr"`
}

func createDeployment(clusterID string, clusterType string,
	obsAddonSpec oashared.ObservabilityAddonSpec,
	hubInfo operatorconfig.HubInfo, allowlist MetricsAllowlist, replicaCount int32) *appsv1.Deployment {
	interval := fmt.Sprint(obsAddonSpec.Interval) + "s"
	if fmt.Sprint(obsAddonSpec.Interval) == "" {
		interval = defaultInterval
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
		"--interval=" + interval,
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
	for _, metrics := range allowlist.NameList {
		commands = append(commands, fmt.Sprintf("--match={__name__=\"%s\"}", metrics))
	}
	for _, match := range allowlist.MatchList {
		commands = append(commands, fmt.Sprintf("--match={%s}", match))
	}
	for k, v := range allowlist.ReNameMap {
		commands = append(commands, fmt.Sprintf("--rename=\"%s=%s\"", k, v))
	}
	for _, rule := range allowlist.RuleList {
		commands = append(commands, fmt.Sprintf("--recordingrule={\"name\":\"%s\",\"query\":\"%s\"}", rule.Record, rule.Expr))
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
							ImagePullPolicy: corev1.PullAlways,
						},
					},
					Volumes: volumes,
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

	list := getMetricsAllowlist(ctx, client)
	deployment := createDeployment(clusterID, clusterType, obsAddonSpec, hubInfo, list, replicaCount)
	found := &appsv1.Deployment{}
	err := client.Get(ctx, types.NamespacedName{Name: metricsCollectorName,
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

func getMetricsAllowlist(ctx context.Context, client client.Client) MetricsAllowlist {
	l := &MetricsAllowlist{}
	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Name: operatorconfig.AllowlistConfigMapName,
		Namespace: namespace}, cm)
	if err != nil {
		log.Error(err, "Failed to get configmap")
	} else {
		if cm.Data != nil {
			err = yaml.Unmarshal([]byte(cm.Data[metricsConfigMapKey]), l)
			if err != nil {
				log.Error(err, "Failed to unmarshal data in configmap")
			}
		}
	}
	return *l
}
