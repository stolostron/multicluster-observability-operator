// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path"
	"slices"

	promconfig "github.com/prometheus/common/config"
	oashared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	obv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/rendering/templates"
	mcoutil "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/resource"
)

const (
	metricsInspectorPort                = 8490
	metricsInspectorObjstoreMountPath   = "/etc/metrics-inspector/objstore"
	metricsInspectorDefaultTLSMountPath = "/etc/thanos/secrets"
	metricsInspectorSATokenMountPath    = "/var/run/secrets/openshift/serviceaccount" // #nosec G101 -- Mount path for bound service account token, not a credential.
	metricsInspectorConfigHashKey       = "observability.open-cluster-management.io/config-hash"
)

// MetricsInspectorEnabled returns true when spec.capabilities.platform.metrics.inspector.enabled is true.
func MetricsInspectorEnabled(cr *obv1beta2.MultiClusterObservability) bool {
	if cr == nil || cr.Spec.Capabilities == nil || cr.Spec.Capabilities.Platform == nil {
		return false
	}
	return cr.Spec.Capabilities.Platform.Metrics.Inspector.Enabled
}

// MetricsInspectorWriteURL returns the internal cluster-local HTTPS URL used by observatorium-api's
// AdditionalWriteEndpoints to mirror incoming Remote Write payloads to the metrics inspector over mTLS.
func MetricsInspectorWriteURL() string {
	return fmt.Sprintf(
		"https://%s.%s.svc:%d/api/v1/write",
		mcoconfig.MetricsInspectorServiceName,
		mcoconfig.GetDefaultNamespace(),
		metricsInspectorPort,
	)
}

// MetricsInspectorWriteEndpoint returns the RemoteWriteEndpointWithSecret entry for observatorium-api
// and the list of secret names that must be mounted into observatorium-api pods for mTLS.
func MetricsInspectorWriteEndpoint() (mcoutil.RemoteWriteEndpointWithSecret, []string) {
	u, _ := url.Parse(MetricsInspectorWriteURL())
	return mcoutil.RemoteWriteEndpointWithSecret{
		Name: mcoconfig.MetricsInspector,
		URL:  promconfig.URL{URL: u},
		HttpClientConfig: &mcoutil.HTTPClientConfigWithSecret{
			TLSConfig: &mcoutil.TLSConfigWithSecret{
				CAFile:     path.Join(mcoutil.MountPath, mcoconfig.ServerCACerts, "ca.crt"),
				CertFile:   path.Join(mcoutil.MountPath, mcoconfig.GrafanaCerts, "tls.crt"),
				KeyFile:    path.Join(mcoutil.MountPath, mcoconfig.GrafanaCerts, "tls.key"),
				ServerName: mcoconfig.ServerCertCN,
			},
		},
	}, []string{mcoconfig.ServerCACerts, mcoconfig.GrafanaCerts}
}

func (r *MCORenderer) newMetricsInspectorRenderer() {
	r.renderMetricsInspectorFns = map[string]rendererutil.RenderFn{
		"Deployment":         r.renderMetricsInspectorDeployment,
		"Service":            r.renderer.RenderNamespace,
		"ServiceAccount":     r.renderMetricsInspectorServiceAccount,
		"ServiceMonitor":     r.renderer.RenderNamespace,
		"ConfigMap":          r.renderer.RenderNamespace,
		"ScrapeConfig":       r.renderer.RenderNamespace,
		"ClusterRole":        r.renderer.RenderClusterRole,
		"ClusterRoleBinding": r.renderer.RenderClusterRoleBinding,
	}
}

// MetricsInspectorResources renders the metrics-inspector templates into unstructured resources.
func (r *MCORenderer) MetricsInspectorResources(ctx context.Context, namespace string, labels map[string]string) ([]*unstructured.Unstructured, error) {
	inspectorTemplates, err := templates.GetOrLoadMetricsInspectorTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return nil, err
	}
	return r.renderMetricsInspectorTemplates(ctx, inspectorTemplates, namespace, labels, false)
}

// MetricsInspectorResourcesForRemoval renders all metrics-inspector templates (including MCOA ScrapeConfig) for undeployment.
func (r *MCORenderer) MetricsInspectorResourcesForRemoval(ctx context.Context, namespace string, labels map[string]string) ([]*unstructured.Unstructured, error) {
	inspectorTemplates, err := templates.GetOrLoadMetricsInspectorTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return nil, err
	}
	return r.renderMetricsInspectorTemplates(ctx, inspectorTemplates, namespace, labels, true)
}

func (r *MCORenderer) renderMetricsInspectorTemplates(
	ctx context.Context,
	inspectorTemplates []*resource.Resource,
	namespace string,
	labels map[string]string,
	forRemoval bool,
) ([]*unstructured.Unstructured, error) {
	uobjs := make([]*unstructured.Unstructured, 0, len(inspectorTemplates))
	for _, template := range inspectorTemplates {
		if !forRemoval && !MCOAPlatformMetricsEnabled(r.cr) && isMCOASpecificResourceKind(template) {
			continue
		}
		render, ok := r.renderMetricsInspectorFns[template.GetKind()]
		if !ok {
			m, err := template.Map()
			if err != nil {
				return nil, err
			}
			uobjs = append(uobjs, &unstructured.Unstructured{Object: m})
			continue
		}
		uobj, err := render(ctx, template.DeepCopy(), namespace, labels)
		if err != nil {
			return nil, err
		}
		if uobj == nil {
			continue
		}
		uobjs = append(uobjs, uobj)
	}
	return uobjs, nil
}

func (r *MCORenderer) renderMetricsInspectorServiceAccount(
	ctx context.Context,
	res *resource.Resource,
	namespace string,
	labels map[string]string,
) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderNamespace(ctx, res, namespace, labels)
	if err != nil {
		return nil, err
	}

	cLabels := maps.Clone(u.GetLabels())
	if cLabels == nil {
		cLabels = make(map[string]string)
	}
	maps.Copy(cLabels, labels)
	u.SetLabels(cLabels)

	if saAnnotations := resolveMetricsInspectorServiceAccountAnnotations(r.cr); len(saAnnotations) > 0 {
		annotations := maps.Clone(u.GetAnnotations())
		if annotations == nil {
			annotations = make(map[string]string)
		}
		maps.Copy(annotations, saAnnotations)
		u.SetAnnotations(annotations)
	}

	return u, nil
}

func (r *MCORenderer) renderMetricsInspectorDeployment(
	ctx context.Context,
	res *resource.Resource,
	namespace string,
	labels map[string]string,
) (*unstructured.Unstructured, error) {
	u, err := r.renderer.RenderNamespace(ctx, res, namespace, labels)
	if err != nil {
		return nil, err
	}

	dep := &appsv1.Deployment{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, dep); err != nil {
		return nil, err
	}

	if operandName := mcoconfig.GetOperandName(mcoconfig.MetricsInspector); operandName != "" {
		dep.Name = operandName
	}

	crLabelKey := mcoconfig.GetCrLabelKey()
	if dep.Labels == nil {
		dep.Labels = make(map[string]string)
	}
	maps.Copy(dep.Labels, labels)
	dep.Labels[crLabelKey] = r.cr.Name

	if dep.Spec.Selector.MatchLabels == nil {
		dep.Spec.Selector.MatchLabels = make(map[string]string)
	}
	dep.Spec.Selector.MatchLabels[crLabelKey] = r.cr.Name

	if dep.Spec.Template.Labels == nil {
		dep.Spec.Template.Labels = make(map[string]string)
	}
	maps.Copy(dep.Spec.Template.Labels, labels)
	dep.Spec.Template.Labels[crLabelKey] = r.cr.Name

	dep.Spec.Replicas = mcoconfig.GetReplicas(mcoconfig.MetricsInspector, r.cr.Spec.InstanceSize, r.cr.Spec.AdvancedConfig)
	dep.Spec.Template.Spec.NodeSelector = r.cr.Spec.NodeSelector
	dep.Spec.Template.Spec.Tolerations = r.cr.Spec.Tolerations
	dep.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
		{Name: mcoconfig.GetImagePullSecret(r.cr.Spec)},
	}

	var inspectorContainer *corev1.Container
	var kubeRbacProxyContainer *corev1.Container
	for i := range dep.Spec.Template.Spec.Containers {
		switch dep.Spec.Template.Spec.Containers[i].Name {
		case mcoconfig.MetricsInspector:
			inspectorContainer = &dep.Spec.Template.Spec.Containers[i]
		case "kube-rbac-proxy":
			kubeRbacProxyContainer = &dep.Spec.Template.Spec.Containers[i]
		}
	}
	if inspectorContainer == nil {
		return nil, fmt.Errorf("no %s container found in the metrics-inspector deployment template", mcoconfig.MetricsInspector)
	}

	defaultImg := mcoconfig.DefaultImgRepository + "/" + mcoconfig.MetricsInspectorImgName + ":" + mcoconfig.DefaultImgTagSuffix
	if replaced, customImg := mcoconfig.ReplaceImage(r.cr.Annotations, defaultImg, mcoconfig.MetricsInspectorKey); replaced {
		defaultImg = customImg
	}
	inspectorContainer.Image = defaultImg
	inspectorContainer.ImagePullPolicy = mcoconfig.GetImagePullPolicy(r.cr.Spec)

	if !mcoconfig.WithoutResourcesRequests(r.cr.GetAnnotations()) {
		inspectorContainer.Resources = mcoconfig.GetResources(mcoconfig.MetricsInspector, r.cr.Spec.InstanceSize, r.cr.Spec.AdvancedConfig)
	} else {
		inspectorContainer.Resources = corev1.ResourceRequirements{}
	}

	receiveWriteURL := fmt.Sprintf(
		"http://%sthanos-receive.%s.svc:19291/api/v1/receive",
		mcoconfig.GetOperandNamePrefix(),
		namespace,
	)
	retArgs := resolveMetricsInspectorRetentionArgs(r.cr)
	objArgs, objMounts, objVolumes, combinedHash, err := r.resolveMetricsInspectorObjectStorage(ctx, namespace)
	if err != nil {
		return nil, err
	}

	inspectorContainer.Args = append(
		inspectorContainer.Args,
		fmt.Sprintf("--estimator.max-items-per-interval=%d", resolveMetricsInspectorMaxItems(r.cr.Spec.InstanceSize)),
		fmt.Sprintf("--remote-write.url=%s", receiveWriteURL),
		fmt.Sprintf("--remote-write.tenant=%s", mcoconfig.GetTenantUID()),
	)
	inspectorContainer.Args = append(inspectorContainer.Args, retArgs...)
	inspectorContainer.Args = append(inspectorContainer.Args, objArgs...)
	inspectorArgs, err := util.SetTLSSecurityConfiguration(ctx, inspectorContainer.Args, "--tls-cipher-suites=", "--tls-min-version=")
	if err != nil {
		return nil, err
	}
	inspectorContainer.Args = inspectorArgs

	inspectorContainer.VolumeMounts = append(inspectorContainer.VolumeMounts, objMounts...)
	dep.Spec.Template.Spec.Volumes = append(dep.Spec.Template.Spec.Volumes, objVolumes...)

	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}
	dep.Spec.Template.Annotations[metricsInspectorConfigHashKey] = combinedHash

	if util.ProxyEnvVarsAreSet() {
		inspectorContainer.Env = []corev1.EnvVar{
			{Name: "HTTP_PROXY", Value: os.Getenv("HTTP_PROXY")},
			{Name: "HTTPS_PROXY", Value: os.Getenv("HTTPS_PROXY")},
			{Name: "NO_PROXY", Value: os.Getenv("NO_PROXY")},
		}
	}

	if kubeRbacProxyContainer != nil {
		if ok, image := mcoconfig.ReplaceImage(r.cr.Annotations, mcoconfig.DefaultImgRepository+"/"+mcoconfig.KubeRBACProxyImgName, mcoconfig.KubeRBACProxyKey); ok {
			kubeRbacProxyContainer.Image = image
		}
		kubeRbacProxyContainer.ImagePullPolicy = mcoconfig.GetImagePullPolicy(r.cr.Spec)

		args, err := util.SetTLSSecurityConfiguration(ctx, kubeRbacProxyContainer.Args, "--tls-cipher-suites=", "--tls-min-version=")
		if err != nil {
			return nil, err
		}
		kubeRbacProxyContainer.Args = args
	}

	uObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dep)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: uObj}, nil
}

func resolveMetricsInspectorServiceAccountAnnotations(mco *obv1beta2.MultiClusterObservability) map[string]string {
	if mco == nil || mco.Spec.AdvancedConfig == nil {
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

// resolveMetricsInspectorMaxItems returns the Bloom filter capacity per sliding interval
// for the given instanceSize, oversized by ~1.5x (2.5x for default) above the nominal
// active series tier. Because saturated Bloom filter shards switch to pass-through mode
// (forwarding all duplicate samples to the mutex-locked HLL maps), this headroom absorbs
// 15-minute cumulative series churn and minor hash imbalance across shards at a cost of
// only 16 bits (2 bytes) of RAM per item.
func resolveMetricsInspectorMaxItems(instanceSize obv1beta2.TShirtSize) int {
	switch instanceSize {
	case mcoconfig.Minimal, mcoconfig.Small:
		return 1500000
	case mcoconfig.Medium:
		return 7500000
	case mcoconfig.Large:
		return 15000000
	case mcoconfig.XLarge:
		return 30000000
	case mcoconfig.TwoXLarge:
		return 75000000
	case mcoconfig.FourXLarge:
		return 150000000
	default:
		return 500000
	}
}

func resolveMetricsInspectorRetentionArgs(mco *obv1beta2.MultiClusterObservability) []string {
	retentionRaw := mcoconfig.RetentionResolutionRaw
	retention5m := mcoconfig.RetentionResolution5m
	retention1h := mcoconfig.RetentionResolution1h
	if mco != nil && mco.Spec.AdvancedConfig != nil && mco.Spec.AdvancedConfig.RetentionConfig != nil {
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

func (r *MCORenderer) resolveMetricsInspectorObjectStorage(
	ctx context.Context,
	namespace string,
) ([]string, []corev1.VolumeMount, []corev1.Volume, string, error) {
	hasher := sha256.New()
	if _, err := hasher.Write([]byte(mcoconfig.MetricsInspectorConfigMapName)); err != nil {
		return nil, nil, nil, "", fmt.Errorf("failed to hash ConfigMap name: %w", err)
	}
	if err := r.hashConfigMapData(ctx, namespace, mcoconfig.MetricsInspectorConfigMapName, hasher); err != nil {
		return nil, nil, nil, "", err
	}

	if r.cr == nil || r.cr.Spec.StorageConfig == nil || r.cr.Spec.StorageConfig.MetricObjectStorage == nil {
		return nil, nil, nil, hex.EncodeToString(hasher.Sum(nil)), nil
	}

	objStorage := r.cr.Spec.StorageConfig.MetricObjectStorage
	if objStorage.Name == "" || objStorage.Key == "" {
		return nil, nil, nil, hex.EncodeToString(hasher.Sum(nil)), nil
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
	if err := r.hashSecretData(ctx, namespace, objStorage.Name, hasher); err != nil {
		return nil, nil, nil, "", err
	}

	if objStorage.TLSSecretName != "" {
		tlsMountPath := objStorage.TLSSecretMountPath
		if tlsMountPath == "" && r.kubeClient != nil {
			derivedPath, err := deriveObjstoreTLSMountPath(ctx, r.kubeClient, namespace, objStorage)
			if err != nil {
				return nil, nil, nil, "", fmt.Errorf("failed to derive object storage TLS mount path: %w", err)
			}
			tlsMountPath = derivedPath
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
		if err := r.hashSecretData(ctx, namespace, objStorage.TLSSecretName, hasher); err != nil {
			return nil, nil, nil, "", err
		}
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

	return extraArgs, extraMounts, extraVolumes, hex.EncodeToString(hasher.Sum(nil)), nil
}

func (r *MCORenderer) hashConfigMapData(ctx context.Context, namespace, configMapName string, hasher io.Writer) error {
	if r.kubeClient != nil {
		cm := &corev1.ConfigMap{}
		err := r.kubeClient.Get(ctx, types.NamespacedName{
			Name:      configMapName,
			Namespace: namespace,
		}, cm)
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to get ConfigMap %s/%s: %w", namespace, configMapName, err)
		}
		if err == nil {
			return writeStringMapToHasher(hasher, cm.Data)
		}
	}

	inspectorTemplates, err := templates.GetOrLoadMetricsInspectorTemplates(templatesutil.GetTemplateRenderer())
	if err != nil {
		return fmt.Errorf("failed to load metrics-inspector templates for ConfigMap %s: %w", configMapName, err)
	}
	for _, tmpl := range inspectorTemplates {
		if tmpl.GetKind() == "ConfigMap" && tmpl.GetName() == configMapName {
			return writeStringMapToHasher(hasher, tmpl.GetDataMap())
		}
	}
	return nil
}

func writeStringMapToHasher(hasher io.Writer, data map[string]string) error {
	for _, k := range slices.Sorted(maps.Keys(data)) {
		if _, err := hasher.Write([]byte(k)); err != nil {
			return fmt.Errorf("failed to hash key %q: %w", k, err)
		}
		if _, err := hasher.Write([]byte(data[k])); err != nil {
			return fmt.Errorf("failed to hash value for key %q: %w", k, err)
		}
	}
	return nil
}

func (r *MCORenderer) hashSecretData(ctx context.Context, namespace, secretName string, hasher io.Writer) error {
	if r.kubeClient == nil {
		return nil
	}
	secret := &corev1.Secret{}
	err := r.kubeClient.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, secret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get Secret %s/%s: %w", namespace, secretName, err)
	}
	for _, k := range slices.Sorted(maps.Keys(secret.Data)) {
		if _, err := hasher.Write([]byte(k)); err != nil {
			return fmt.Errorf("failed to hash key %q: %w", k, err)
		}
		if _, err := hasher.Write(secret.Data[k]); err != nil {
			return fmt.Errorf("failed to hash value for key %q: %w", k, err)
		}
	}
	return nil
}

func deriveObjstoreTLSMountPath(
	ctx context.Context,
	c client.Client,
	namespace string,
	objectStorage *oashared.PreConfiguredStorage,
) (string, error) {
	found := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{Name: objectStorage.Name, Namespace: namespace}, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	data, ok := found.Data[objectStorage.Key]
	if !ok {
		return "", nil
	}
	var objectConf mcoconfig.ObjectStorgeConf
	if err := yaml.Unmarshal(data, &objectConf); err != nil {
		return "", err
	}
	caFile := objectConf.Config.HTTPConfig.TLSConfig.CAFile
	if caFile == "" {
		return "", nil
	}
	return path.Dir(caFile), nil
}
