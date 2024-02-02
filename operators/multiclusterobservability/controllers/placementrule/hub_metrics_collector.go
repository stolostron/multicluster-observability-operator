package placementrule

import (
	"context"
	"fmt"
	"os"
	"strconv"

	ocinfrav1 "github.com/openshift/api/config/v1"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	caMounthPath         = "/etc/serving-certs-ca-bundle"
	caVolName            = "serving-certs-ca-bundle"
	mtlsCertName         = "observability-controller-open-cluster-management.io-observability-signer-client-cert"
	mtlsCaName           = "observability-managed-cluster-certs"
	metricsCollectorName = "hub-metrics-collector-deployment"
	caConfigmapName      = "metrics-collector-serving-certs-ca-bundle"
	restartLabel         = "cert/time-restarted"
	defaultInterval      = "30s"
	limitBytes           = 1073741824
	selectorKey          = "component"
	selectorValue        = "metrics-collector"
)

var (
	ocpPromURL           = "https://prometheus-k8s.openshift-monitoring.svc:9091"
	uwlPromURL           = "https://prometheus-user-workload.openshift-user-workload-monitoring.svc:9092"
	uwlQueryURL          = "https://thanos-querier.openshift-monitoring.svc:9091"
	promURL              = "https://prometheus-k8s:9091"
	installPrometheus, _ = strconv.ParseBool(os.Getenv(operatorconfig.InstallPrometheus))
	ownerLabelKey        = "owner"
	ownerLabelValue      = "multicluster-observability-operator"
)

type HubCollectorParams struct {
	isUWL        bool
	clusterID    string
	clusterType  string
	obsAddonSpec oashared.ObservabilityAddonSpec
	hubInfo      operatorconfig.HubInfo
	allowlist    operatorconfig.MetricsAllowlist
	nodeSelector map[string]string
	tolerations  []corev1.Toleration
	CABundle     string
	replicaCount int32
}

func getClusterID(ctx context.Context, c client.Client) (string, error) {
	clusterVersion := &ocinfrav1.ClusterVersion{}
	if err := c.Get(ctx, types.NamespacedName{Name: "version"}, clusterVersion); err != nil {
		log.Error(err, "Failed to get clusterVersion")
		return "", err
	}

	return string(clusterVersion.Spec.ClusterID), nil
}

func isSingleNode(ctx context.Context, c client.Client) (bool, error) {
	nodes := &corev1.NodeList{}
	opts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"node-role.kubernetes.io/master": ""}),
	}
	err := c.List(ctx, nodes, opts)
	if err != nil {
		log.Error(err, "Failed to get node list")
		return false, err
	}
	if len(nodes.Items) == 1 {
		return true, nil
	}
	return false, nil
}

func isSNO(ctx context.Context, c client.Client) (bool, error) {
	infraConfig := &ocinfrav1.Infrastructure{}
	if err := c.Get(ctx, types.NamespacedName{Name: "cluster"}, infraConfig); err != nil {
		log.Info("No OCP infrastructure found, determine SNO by checking node size")
		return isSingleNode(ctx, c)
	}
	if infraConfig.Status.ControlPlaneTopology == ocinfrav1.SingleReplicaTopologyMode {
		return true, nil
	}

	return false, nil
}

func getCommands(params HubCollectorParams) []string {
	interval := fmt.Sprint(params.obsAddonSpec.Interval) + "s"
	if fmt.Sprint(params.obsAddonSpec.Interval) == "" {
		interval = defaultInterval
	}
	evaluateInterval := "30s"
	if params.obsAddonSpec.Interval < 30 {
		evaluateInterval = interval
	}

	caFile := caMounthPath + "/service-ca.crt"
	clusterID := params.clusterID
	if params.clusterType == "ocp3" {
		clusterID = params.hubInfo.ClusterName
		// deprecated ca bundle, only used for ocp 3.11 env
		caFile = "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
	}
	commands := []string{
		"/usr/bin/metrics-collector",
		"--from=$(FROM)",
		"--from-query=$(FROM_QUERY)",
		"--to-upload=$(TO)",
		"--to-upload-ca=/tlscerts/ca/ca.crt",
		"--to-upload-cert=/tlscerts/certs/tls.crt",
		"--to-upload-key=/tlscerts/certs/tls.key",
		"--interval=" + interval,
		"--evaluate-interval=" + evaluateInterval,
		"--limit-bytes=" + strconv.Itoa(limitBytes),
		fmt.Sprintf("--label=\"cluster=%s\"", params.hubInfo.ClusterName),
		fmt.Sprintf("--label=\"clusterID=%s\"", clusterID),
	}
	commands = append(commands, "--from-token-file=/var/run/secrets/kubernetes.io/serviceaccount/token")
	if !installPrometheus {
		commands = append(commands, "--from-ca-file="+caFile)
	}
	if params.clusterType != "" {
		commands = append(commands, fmt.Sprintf("--label=\"clusterType=%s\"", params.clusterType))
	}

	//dynamicMetricList := map[string]bool{}
	//for _, group := range params.allowlist.CollectRuleGroupList {
	//	if group.Selector.MatchExpression != nil {
	//		for _, expr := range group.Selector.MatchExpression {
	//			if !evluateMatchExpression(expr, clusterID, params.clusterType, params.obsAddonSpec, params.hubInfo,
	//				params.allowlist, params.nodeSelector, params.tolerations, params.replicaCount) {
	//				continue
	//			}
	//			for _, rule := range group.CollectRuleList {
	//				matchList := []string{}
	//				for _, match := range rule.Metrics.MatchList {
	//					matchList = append(matchList, `"`+strings.ReplaceAll(match, `"`, `\"`)+`"`)
	//					if name := getNameInMatch(match); name != "" {
	//						dynamicMetricList[name] = false
	//					}
	//				}
	//				for _, name := range rule.Metrics.NameList {
	//					dynamicMetricList[name] = false
	//				}
	//				matchListStr := "[" + strings.Join(matchList, ",") + "]"
	//				nameListStr := `["` + strings.Join(rule.Metrics.NameList, `","`) + `"]`
	//				commands = append(
	//					commands,
	//					fmt.Sprintf("--collectrule={\"name\":\"%s\",\"expr\":\"%s\",\"for\":\"%s\",\"names\":%v,\"matches\":%v}",
	//						rule.Collect, rule.Expr, rule.For, nameListStr, matchListStr),
	//				)
	//			}
	//		}
	//	}
	//}

	//for _, metrics := range params.allowlist.NameList {
	//	if _, ok := dynamicMetricList[metrics]; !ok {
	//		commands = append(commands, fmt.Sprintf("--match={__name__=\"%s\"}", metrics))
	//	}
	//}
	//for _, match := range params.allowlist.MatchList {
	//	if name := getNameInMatch(match); name != "" {
	//		if _, ok := dynamicMetricList[name]; ok {
	//			continue
	//		}
	//	}
	//	commands = append(commands, fmt.Sprintf("--match={%s}", match))
	//}
	//
	//renamekeys := make([]string, 0, len(params.allowlist.RenameMap))
	//for k := range params.allowlist.RenameMap {
	//	renamekeys = append(renamekeys, k)
	//}
	//sort.Strings(renamekeys)
	//for _, k := range renamekeys {
	//	commands = append(commands, fmt.Sprintf("--rename=\"%s=%s\"", k, params.allowlist.RenameMap[k]))
	//}
	//for _, rule := range params.allowlist.RecordingRuleList {
	//	commands = append(
	//		commands,
	//		fmt.Sprintf("--recordingrule={\"name\":\"%s\",\"query\":\"%s\"}", rule.Record, rule.Expr),
	//	)
	//}
	return commands
}

func int32Ptr(i int32) *int32 { return &i }

// CreateMetricsCollector creates the metrics collector for hub cluster
func GenerateMetricsCollectorForHub(ctx context.Context, mcoInstance *mcov1beta2.MultiClusterObservability, params HubCollectorParams) (*appsv1.Deployment, error) {
	hubInfo := &operatorconfig.HubInfo{}
	err := yaml.Unmarshal(hubInfoSecret.Data[operatorconfig.HubInfoSecretKey], &hubInfo)
	if err != nil {
		log.Error(err, "Failed to unmarshal hub info")
		return nil, err
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

	if params.clusterID != "" {
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

	commands := getCommands(params)

	from := promURL
	if !installPrometheus {
		from = ocpPromURL
		//if params.isUWL {
		//	from = uwlPromURL
		//}
	}
	fromQuery := from
	//if params.isUWL {
	//	fromQuery = uwlQueryURL
	//}
	name := metricsCollectorName
	//if params.isUWL {
	//	name = uwlMetricsCollectorName
	//}
	metricsCollectorDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: config.GetDefaultNamespace(),
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
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
					//ServiceAccountName: serviceAccountName,
					Containers: []corev1.Container{
						{
							Name:  "metrics-collector",
							Image: imageListConfigMap.Data[operatorconfig.MetricsCollectorKey],
							//Image:   rendering.Images[operatorconfig.MetricsCollectorKey],
							Command: commands,
							Env: []corev1.EnvVar{
								{
									Name:  "FROM",
									Value: from,
								},
								{
									Name:  "FROM_QUERY",
									Value: fromQuery,
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
					NodeSelector: params.nodeSelector,
					Tolerations:  params.tolerations,
				},
			},
		},
	}

	// No proxy config for hub
	//if params.httpProxy != "" || params.httpsProxy != "" || params.noProxy != "" {
	//	metricsCollectorDep.Spec.Template.Spec.Containers[0].Env = append(metricsCollectorDep.Spec.Template.Spec.Containers[0].Env,
	//		corev1.EnvVar{
	//			Name:  "HTTP_PROXY",
	//			Value: params.httpProxy,
	//		},
	//		corev1.EnvVar{
	//			Name:  "HTTPS_PROXY",
	//			Value: params.httpsProxy,
	//		},
	//		corev1.EnvVar{
	//			Name:  "NO_PROXY",
	//			Value: params.noProxy,
	//		})
	//}
	//if params.httpsProxy != "" && params.CABundle != "" {
	//	metricsCollectorDep.Spec.Template.Spec.Containers[0].Env = append(metricsCollectorDep.Spec.Template.Spec.Containers[0].Env,
	//		corev1.EnvVar{
	//			Name:  "HTTPS_PROXY_CA_BUNDLE",
	//			Value: params.CABundle,
	//		})
	//}

	if mcoInstance.Spec.ObservabilityAddonSpec.Resources != nil {
		metricsCollectorDep.Spec.Template.Spec.Containers[0].Resources = *mcoInstance.Spec.ObservabilityAddonSpec.Resources
	}
	return metricsCollectorDep, nil
}
