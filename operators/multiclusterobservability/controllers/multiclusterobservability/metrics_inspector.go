// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package multiclusterobservability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"net/url"
	"os"
	"reflect"
	"slices"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	promconfig "github.com/prometheus/common/config"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoutil "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	commonutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	metricsInspectorPort                = 8490
	metricsInspectorConfigFileKey       = "config.yaml"
	metricsInspectorMountPath           = "/etc/metrics-inspector"
	metricsInspectorObjstoreMountPath   = "/etc/metrics-inspector/objstore"
	metricsInspectorDefaultTLSMountPath = "/etc/thanos/secrets"
	metricsInspectorSATokenMountPath    = "/var/run/secrets/openshift/serviceaccount" // #nosec G101 -- Mount path for bound service account token, not a credential.
	metricsInspectorConfigHashKey       = "observability.open-cluster-management.io/config-hash"
)

// defaultMetricsInspectorConfigYAML defines the 4 deterministic cardinality streams
// sized for up to 2,600 managed clusters and 150-200 active namespaces per cluster.
// The sliding window interval is controlled via the --estimator.interval CLI flag (15m default).
const defaultMetricsInspectorConfigYAML = `max_items_per_interval: 15000000
streams:
  - group_by: [cluster]
    hll_precision: 14
    calculate_churn: true
    group_limit: 5000
    min_cardinality: 0
  - group_by: [__name__]
    hll_precision: 14
    calculate_churn: true
    group_limit: 10000
    min_cardinality: 0
  - group_by: [cluster, namespace]
    hll_precision: 12
    calculate_churn: true
    group_limit: 500000
    min_cardinality: 25
  - group_by: [cluster, __name__]
    hll_precision: 12
    calculate_churn: false
    group_limit: 650000
    min_cardinality: 25
`

// MetricsInspectorWriteURL returns the internal cluster-local URL used by observatorium-api's
// AdditionalWriteEndpoints to mirror incoming Remote Write payloads to the metrics inspector.
func MetricsInspectorWriteURL() string {
	return fmt.Sprintf(
		"http://%s.%s.svc:%d/api/v1/write",
		mcoconfig.MetricsInspectorServiceName,
		mcoconfig.GetDefaultNamespace(),
		metricsInspectorPort,
	)
}

// MetricsInspectorWriteEndpoint returns the RemoteWriteEndpointWithSecret entry for observatorium-api.
func MetricsInspectorWriteEndpoint() mcoutil.RemoteWriteEndpointWithSecret {
	u, _ := url.Parse(MetricsInspectorWriteURL())
	return mcoutil.RemoteWriteEndpointWithSecret{
		Name: mcoconfig.MetricsInspector,
		URL:  promconfig.URL{URL: u},
	}
}

// GenerateMetricsInspectorResources reconciles the ConfigMap, ServiceAccount, Service, ServiceMonitor, and 1-replica Deployment
// for the Hub metrics inspector.
func GenerateMetricsInspectorResources(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
) (*ctrl.Result, error) {
	configHash, err := reconcileMetricsInspectorConfigMap(ctx, c, scheme, mco)
	if err != nil {
		return &ctrl.Result{}, fmt.Errorf("failed to reconcile metrics inspector configmap: %w", err)
	}

	if err := reconcileMetricsInspectorServiceAccount(ctx, c, scheme, mco); err != nil {
		return &ctrl.Result{}, fmt.Errorf("failed to reconcile metrics inspector serviceaccount: %w", err)
	}

	if err := reconcileMetricsInspectorService(ctx, c, scheme, mco); err != nil {
		return &ctrl.Result{}, fmt.Errorf("failed to reconcile metrics inspector service: %w", err)
	}

	if err := reconcileMetricsInspectorServiceMonitor(ctx, c, scheme, mco); err != nil {
		return &ctrl.Result{}, fmt.Errorf("failed to reconcile metrics inspector servicemonitor: %w", err)
	}

	if err := reconcileMetricsInspectorDeployment(ctx, c, scheme, mco, configHash); err != nil {
		return &ctrl.Result{}, fmt.Errorf("failed to reconcile metrics inspector deployment: %w", err)
	}

	return nil, nil
}

func metricsInspectorLabels(mco *mcov1beta2.MultiClusterObservability) map[string]string {
	return map[string]string{
		"app.kubernetes.io/component": mcoconfig.MetricsInspector,
		"app.kubernetes.io/name":      mcoconfig.MetricsInspectorServiceName,
		"app.kubernetes.io/part-of":   "multicluster-observability-operator",
		mcoconfig.GetCrLabelKey():     mco.Name,
	}
}

func reconcileMetricsInspectorConfigMap(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
) (string, error) {
	sum := sha256.Sum256([]byte(defaultMetricsInspectorConfigYAML))
	configHash := hex.EncodeToString(sum[:])

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoconfig.MetricsInspectorConfigMapName,
			Namespace: mcoconfig.GetDefaultNamespace(),
			Labels:    metricsInspectorLabels(mco),
		},
		Data: map[string]string{
			metricsInspectorConfigFileKey: defaultMetricsInspectorConfigYAML,
		},
	}

	if err := controllerutil.SetControllerReference(mco, cm, scheme); err != nil {
		return "", err
	}

	found := &corev1.ConfigMap{}
	err := c.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Creating metrics inspector ConfigMap", "name", cm.Name, "namespace", cm.Namespace)
			if err := c.Create(ctx, cm); err != nil {
				return "", err
			}
			return configHash, nil
		}
		return "", err
	}

	if !maps.Equal(found.Data, cm.Data) || !maps.Equal(found.Labels, cm.Labels) {
		log.Info("Updating metrics inspector ConfigMap", "name", cm.Name, "namespace", cm.Namespace)
		updated := found.DeepCopy()
		updated.Data = cm.Data
		updated.Labels = cm.Labels
		if err := c.Update(ctx, updated); err != nil {
			return "", err
		}
	}

	return configHash, nil
}

func resolveMetricsInspectorServiceAccountAnnotations(mco *mcov1beta2.MultiClusterObservability) map[string]string {
	if mco.Spec.AdvancedConfig == nil {
		return nil
	}
	if mco.Spec.AdvancedConfig.Store != nil && len(mco.Spec.AdvancedConfig.Store.ServiceAccountAnnotations) > 0 {
		return maps.Clone(mco.Spec.AdvancedConfig.Store.ServiceAccountAnnotations)
	}
	if mco.Spec.AdvancedConfig.Compact != nil && len(mco.Spec.AdvancedConfig.Compact.ServiceAccountAnnotations) > 0 {
		return maps.Clone(mco.Spec.AdvancedConfig.Compact.ServiceAccountAnnotations)
	}
	if mco.Spec.AdvancedConfig.Receive != nil && len(mco.Spec.AdvancedConfig.Receive.ServiceAccountAnnotations) > 0 {
		return maps.Clone(mco.Spec.AdvancedConfig.Receive.ServiceAccountAnnotations)
	}
	return nil
}

func reconcileMetricsInspectorServiceAccount(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        mcoconfig.MetricsInspectorServiceName,
			Namespace:   mcoconfig.GetDefaultNamespace(),
			Labels:      metricsInspectorLabels(mco),
			Annotations: resolveMetricsInspectorServiceAccountAnnotations(mco),
		},
	}

	if err := controllerutil.SetControllerReference(mco, sa, scheme); err != nil {
		return err
	}

	found := &corev1.ServiceAccount{}
	err := c.Get(ctx, types.NamespacedName{Name: sa.Name, Namespace: sa.Namespace}, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Creating metrics inspector ServiceAccount", "name", sa.Name, "namespace", sa.Namespace)
			return c.Create(ctx, sa)
		}
		return err
	}

	if !maps.Equal(found.Labels, sa.Labels) || !maps.Equal(found.Annotations, sa.Annotations) {
		log.Info("Updating metrics inspector ServiceAccount", "name", sa.Name, "namespace", sa.Namespace)
		updated := found.DeepCopy()
		updated.Labels = sa.Labels
		updated.Annotations = sa.Annotations
		return c.Update(ctx, updated)
	}

	return nil
}

func reconcileMetricsInspectorService(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
) error {
	labels := metricsInspectorLabels(mco)
	selector := map[string]string{
		"app.kubernetes.io/component": mcoconfig.MetricsInspector,
		"app.kubernetes.io/name":      mcoconfig.MetricsInspectorServiceName,
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoconfig.MetricsInspectorServiceName,
			Namespace: mcoconfig.GetDefaultNamespace(),
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       metricsInspectorPort,
					TargetPort: intstr.FromInt32(metricsInspectorPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(mco, svc, scheme); err != nil {
		return err
	}

	found := &corev1.Service{}
	err := c.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Creating metrics inspector Service", "name", svc.Name, "namespace", svc.Namespace)
			return c.Create(ctx, svc)
		}
		return err
	}

	if !maps.Equal(found.Labels, svc.Labels) || !maps.Equal(found.Spec.Selector, svc.Spec.Selector) || !reflect.DeepEqual(found.Spec.Ports, svc.Spec.Ports) {
		log.Info("Updating metrics inspector Service", "name", svc.Name, "namespace", svc.Namespace)
		updated := found.DeepCopy()
		updated.Labels = svc.Labels
		updated.Spec.Selector = svc.Spec.Selector
		updated.Spec.Ports = svc.Spec.Ports
		return c.Update(ctx, updated)
	}

	return nil
}

func reconcileMetricsInspectorServiceMonitor(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
) error {
	if _, _, err := scheme.ObjectKinds(&monitoringv1.ServiceMonitor{}); err != nil {
		return nil
	}

	labels := metricsInspectorLabels(mco)
	selectorLabels := map[string]string{
		"app.kubernetes.io/component": mcoconfig.MetricsInspector,
		"app.kubernetes.io/name":      mcoconfig.MetricsInspectorServiceName,
	}

	sm := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoconfig.MetricsInspectorServiceName,
			Namespace: mcoconfig.GetDefaultNamespace(),
			Labels:    labels,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{mcoconfig.GetDefaultNamespace()},
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Port:     "http",
					Path:     "/metrics",
					Interval: monitoringv1.Duration("60s"),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(mco, sm, scheme); err != nil {
		return err
	}

	found := &monitoringv1.ServiceMonitor{}
	err := c.Get(ctx, types.NamespacedName{Name: sm.Name, Namespace: sm.Namespace}, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Creating metrics inspector ServiceMonitor", "name", sm.Name, "namespace", sm.Namespace)
			return c.Create(ctx, sm)
		}
		return err
	}

	if !equality.Semantic.DeepEqual(found.Spec, sm.Spec) || !maps.Equal(found.Labels, sm.Labels) {
		log.Info("Updating metrics inspector ServiceMonitor", "name", sm.Name, "namespace", sm.Namespace)
		updated := found.DeepCopy()
		updated.Labels = sm.Labels
		updated.Spec = sm.Spec
		return c.Update(ctx, updated)
	}

	return nil
}

func resolveMetricsInspectorRetentionArgs(mco *mcov1beta2.MultiClusterObservability) []string {
	retentionRaw := mcoconfig.RetentionResolutionRaw
	retention5m := mcoconfig.RetentionResolution5m
	retention1h := mcoconfig.RetentionResolution1h
	if mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.RetentionConfig != nil {
		rc := mco.Spec.AdvancedConfig.RetentionConfig
		if rc.RetentionResolutionRaw != "" {
			retentionRaw = rc.RetentionResolutionRaw
		}
		if rc.RetentionResolution5m != "" {
			retention5m = rc.RetentionResolution5m
		}
		if rc.RetentionResolution1h != "" {
			retention1h = rc.RetentionResolution1h
		}
	}
	return []string{
		fmt.Sprintf("--retention.resolution-raw=%s", retentionRaw),
		fmt.Sprintf("--retention.resolution-5m=%s", retention5m),
		fmt.Sprintf("--retention.resolution-1h=%s", retention1h),
	}
}

func resolveMetricsInspectorObjectStorage(
	ctx context.Context,
	c client.Client,
	mco *mcov1beta2.MultiClusterObservability,
	baseConfigHash string,
) ([]string, []corev1.VolumeMount, []corev1.Volume, string) {
	hasher := sha256.New()
	hasher.Write([]byte(baseConfigHash))

	if mco.Spec.StorageConfig == nil || mco.Spec.StorageConfig.MetricObjectStorage == nil {
		return nil, nil, nil, baseConfigHash
	}

	objStorage := mco.Spec.StorageConfig.MetricObjectStorage
	if objStorage.Name == "" || objStorage.Key == "" {
		return nil, nil, nil, baseConfigHash
	}

	extraArgs := []string{
		fmt.Sprintf("--objstore.config-file=%s/%s", metricsInspectorObjstoreMountPath, objStorage.Key),
		"--bucket-inspect.interval=15m",
	}
	extraMounts := make([]corev1.VolumeMount, 0, 3)
	extraMounts = append(extraMounts, corev1.VolumeMount{
		Name:      "objstore-secret",
		MountPath: metricsInspectorObjstoreMountPath,
		ReadOnly:  true,
	})
	extraVolumes := make([]corev1.Volume, 0, 3)
	extraVolumes = append(extraVolumes, corev1.Volume{
		Name: "objstore-secret",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: objStorage.Name,
			},
		},
	})
	hashSecretData(ctx, c, objStorage.Name, hasher)

	if objStorage.TLSSecretName != "" {
		tlsMountPath := objStorage.TLSSecretMountPath
		if tlsMountPath == "" {
			if derivedPath, err := getTLSSecretMountPath(c, objStorage); err == nil && derivedPath != "" {
				tlsMountPath = derivedPath
			}
		}
		if tlsMountPath == "" {
			tlsMountPath = fmt.Sprintf("%s/%s", metricsInspectorDefaultTLSMountPath, objStorage.TLSSecretName)
		}
		extraMounts = append(extraMounts, corev1.VolumeMount{
			Name:      "objstore-tls-secret",
			MountPath: tlsMountPath,
			ReadOnly:  true,
		})
		extraVolumes = append(extraVolumes, corev1.Volume{
			Name: "objstore-tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: objStorage.TLSSecretName,
				},
			},
		})
		hashSecretData(ctx, c, objStorage.TLSSecretName, hasher)
	}

	if objStorage.ServiceAccountProjection {
		extraMounts = append(extraMounts, corev1.VolumeMount{
			Name:      "bound-sa-token",
			MountPath: metricsInspectorSATokenMountPath,
			ReadOnly:  true,
		})
		extraVolumes = append(extraVolumes, corev1.Volume{
			Name: "bound-sa-token",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Path:              "token",
								Audience:          "openshift",
								ExpirationSeconds: ptr.To(int64(3600)),
							},
						},
					},
				},
			},
		})
	}

	sum := hasher.Sum(nil)
	return extraArgs, extraMounts, extraVolumes, hex.EncodeToString(sum)
}

func hashSecretData(ctx context.Context, c client.Client, secretName string, hasher interface{ Write([]byte) (int, error) }) {
	secret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: mcoconfig.GetDefaultNamespace(),
	}, secret)
	if err != nil {
		return
	}
	keys := slices.Sorted(maps.Keys(secret.Data))
	for _, k := range keys {
		_, _ = hasher.Write([]byte(k))
		_, _ = hasher.Write(secret.Data[k])
	}
}

func reconcileMetricsInspectorDeployment(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
	configHash string,
) error {
	deployName := mcoconfig.GetOperandName(mcoconfig.MetricsInspector)
	if deployName == "" {
		deployName = mcoconfig.MetricsInspectorServiceName
	}

	labels := metricsInspectorLabels(mco)
	selectorLabels := map[string]string{
		"app.kubernetes.io/component": mcoconfig.MetricsInspector,
		"app.kubernetes.io/name":      mcoconfig.MetricsInspectorServiceName,
	}

	defaultImg := mcoconfig.DefaultImgRepository + "/" + mcoconfig.MetricsInspectorImgName + ":" + mcoconfig.DefaultImgTagSuffix
	if replaced, customImg := mcoconfig.ReplaceImage(mco.Annotations, defaultImg, mcoconfig.MetricsInspectorKey); replaced {
		defaultImg = customImg
	}

	var resources corev1.ResourceRequirements
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		resources = mcoconfig.GetResources(mcoconfig.MetricsInspector, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	}

	replicas := mcoconfig.GetReplicas(mcoconfig.MetricsInspector, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	receiveWriteURL := fmt.Sprintf(
		"http://%sthanos-receive.%s.svc:19291/api/v1/receive",
		mcoconfig.GetOperandNamePrefix(),
		mcoconfig.GetDefaultNamespace(),
	)

	retArgs := resolveMetricsInspectorRetentionArgs(mco)
	objArgs, objMounts, objVolumes, combinedHash := resolveMetricsInspectorObjectStorage(ctx, c, mco, configHash)

	args := make([]string, 0, 7+len(retArgs)+len(objArgs))
	args = append(args,
		fmt.Sprintf("--config.file=%s/%s", metricsInspectorMountPath, metricsInspectorConfigFileKey),
		"--estimator.interval=15m",
		fmt.Sprintf("--web.listen-address=:%d", metricsInspectorPort),
		fmt.Sprintf("--remote-write.url=%s", receiveWriteURL),
		fmt.Sprintf("--remote-write.tenant=%s", mcoconfig.GetTenantUID()),
		"--remote-write.interval=60s",
		"--remote-write.initial-delay=5m",
	)
	args = append(args, retArgs...)
	args = append(args, objArgs...)

	volumeMounts := make([]corev1.VolumeMount, 0, 1+len(objMounts))
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "config",
		MountPath: metricsInspectorMountPath,
		ReadOnly:  true,
	})
	volumeMounts = append(volumeMounts, objMounts...)

	volumes := make([]corev1.Volume, 0, 1+len(objVolumes))
	volumes = append(volumes, corev1.Volume{
		Name: "config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: mcoconfig.MetricsInspectorConfigMapName,
				},
			},
		},
	})
	volumes = append(volumes, objVolumes...)

	var envVars []corev1.EnvVar
	if commonutil.ProxyEnvVarsAreSet() {
		envVars = []corev1.EnvVar{
			{Name: "HTTP_PROXY", Value: os.Getenv("HTTP_PROXY")},
			{Name: "HTTPS_PROXY", Value: os.Getenv("HTTPS_PROXY")},
			{Name: "NO_PROXY", Value: os.Getenv("NO_PROXY")},
		}
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: mcoconfig.GetDefaultNamespace(),
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						metricsInspectorConfigHashKey: combinedHash,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: mcoconfig.MetricsInspectorServiceName,
					NodeSelector:       mco.Spec.NodeSelector,
					Tolerations:        mco.Spec.Tolerations,
					ImagePullSecrets: []corev1.LocalObjectReference{
						{Name: mcoconfig.GetImagePullSecret(mco.Spec)},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
					},
					Containers: []corev1.Container{
						{
							Name:            mcoconfig.MetricsInspector,
							Image:           defaultImg,
							ImagePullPolicy: mcoconfig.GetImagePullPolicy(mco.Spec),
							Args:            args,
							Env:             envVars,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: metricsInspectorPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources:    resources,
							VolumeMounts: volumeMounts,
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt32(metricsInspectorPort),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       30,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromInt32(metricsInspectorPort),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       15,
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								ReadOnlyRootFilesystem:   ptr.To(true),
								RunAsNonRoot:             ptr.To(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(mco, deploy, scheme); err != nil {
		return err
	}

	found := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Creating metrics inspector Deployment", "name", deploy.Name, "namespace", deploy.Namespace)
			return c.Create(ctx, deploy)
		}
		return err
	}

	if !equality.Semantic.DeepEqual(found.Spec, deploy.Spec) || !maps.Equal(found.Labels, deploy.Labels) {
		log.Info("Updating metrics inspector Deployment", "name", deploy.Name, "namespace", deploy.Namespace)
		updated := found.DeepCopy()
		updated.Labels = deploy.Labels
		updated.Spec = deploy.Spec
		return c.Update(ctx, updated)
	}

	return nil
}
