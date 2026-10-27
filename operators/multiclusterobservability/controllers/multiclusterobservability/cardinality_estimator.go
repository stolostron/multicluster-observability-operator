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
	"reflect"

	promconfig "github.com/prometheus/common/config"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	mcoutil "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
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
	cardinalityEstimatorPort          = 8490
	cardinalityEstimatorConfigFileKey = "config.yaml"
	cardinalityEstimatorMountPath     = "/etc/cardinality-estimator"
	cardinalityEstimatorConfigHashKey = "observability.open-cluster-management.io/config-hash"
)

// defaultCardinalityEstimatorConfigYAML defines the 4 deterministic cardinality streams
// sized for up to 2,600 managed clusters and 150-200 active namespaces per cluster.
const defaultCardinalityEstimatorConfigYAML = `max_items_per_interval: 15000000
streams:
  - interval: 10m
    group_by: [cluster]
    hll_precision: 14
    calculate_churn: true
    group_limit: 5000
    min_cardinality: 0
  - interval: 10m
    group_by: [__name__]
    hll_precision: 14
    calculate_churn: true
    group_limit: 10000
    min_cardinality: 0
  - interval: 10m
    group_by: [cluster, namespace]
    hll_precision: 12
    calculate_churn: true
    group_limit: 500000
    min_cardinality: 25
  - interval: 10m
    group_by: [cluster, __name__]
    hll_precision: 12
    calculate_churn: false
    group_limit: 650000
    min_cardinality: 25
`

// CardinalityEstimatorWriteURL returns the internal cluster-local URL used by observatorium-api's
// AdditionalWriteEndpoints to mirror incoming Remote Write payloads to the cardinality estimator.
func CardinalityEstimatorWriteURL() string {
	return fmt.Sprintf(
		"http://%s.%s.svc:%d/api/v1/write",
		mcoconfig.CardinalityEstimatorServiceName,
		mcoconfig.GetDefaultNamespace(),
		cardinalityEstimatorPort,
	)
}

// CardinalityEstimatorWriteEndpoint returns the RemoteWriteEndpointWithSecret entry for observatorium-api.
func CardinalityEstimatorWriteEndpoint() mcoutil.RemoteWriteEndpointWithSecret {
	u, _ := url.Parse(CardinalityEstimatorWriteURL())
	return mcoutil.RemoteWriteEndpointWithSecret{
		Name: mcoconfig.CardinalityEstimator,
		URL:  promconfig.URL{URL: u},
	}
}

// GenerateCardinalityEstimatorResources reconciles the ConfigMap, Service, and 1-replica Deployment
// for the Hub cardinality estimator.
func GenerateCardinalityEstimatorResources(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
) (*ctrl.Result, error) {
	configHash, err := reconcileCardinalityEstimatorConfigMap(ctx, c, scheme, mco)
	if err != nil {
		return &ctrl.Result{}, fmt.Errorf("failed to reconcile cardinality estimator configmap: %w", err)
	}

	if err := reconcileCardinalityEstimatorService(ctx, c, scheme, mco); err != nil {
		return &ctrl.Result{}, fmt.Errorf("failed to reconcile cardinality estimator service: %w", err)
	}

	if err := reconcileCardinalityEstimatorDeployment(ctx, c, scheme, mco, configHash); err != nil {
		return &ctrl.Result{}, fmt.Errorf("failed to reconcile cardinality estimator deployment: %w", err)
	}

	return nil, nil
}

func cardinalityEstimatorLabels(mco *mcov1beta2.MultiClusterObservability) map[string]string {
	return map[string]string{
		"app.kubernetes.io/component": mcoconfig.CardinalityEstimator,
		"app.kubernetes.io/name":      mcoconfig.CardinalityEstimatorServiceName,
		"app.kubernetes.io/part-of":   "multicluster-observability-operator",
		mcoconfig.GetCrLabelKey():     mco.Name,
	}
}

func reconcileCardinalityEstimatorConfigMap(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
) (string, error) {
	sum := sha256.Sum256([]byte(defaultCardinalityEstimatorConfigYAML))
	configHash := hex.EncodeToString(sum[:])

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoconfig.CardinalityEstimatorConfigMapName,
			Namespace: mcoconfig.GetDefaultNamespace(),
			Labels:    cardinalityEstimatorLabels(mco),
		},
		Data: map[string]string{
			cardinalityEstimatorConfigFileKey: defaultCardinalityEstimatorConfigYAML,
		},
	}

	if err := controllerutil.SetControllerReference(mco, cm, scheme); err != nil {
		return "", err
	}

	found := &corev1.ConfigMap{}
	err := c.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, found)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("Creating cardinality estimator ConfigMap", "name", cm.Name, "namespace", cm.Namespace)
			if err := c.Create(ctx, cm); err != nil {
				return "", err
			}
			return configHash, nil
		}
		return "", err
	}

	if !maps.Equal(found.Data, cm.Data) || !maps.Equal(found.Labels, cm.Labels) {
		log.Info("Updating cardinality estimator ConfigMap", "name", cm.Name, "namespace", cm.Namespace)
		updated := found.DeepCopy()
		updated.Data = cm.Data
		updated.Labels = cm.Labels
		if err := c.Update(ctx, updated); err != nil {
			return "", err
		}
	}

	return configHash, nil
}

func reconcileCardinalityEstimatorService(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
) error {
	labels := cardinalityEstimatorLabels(mco)
	selector := map[string]string{
		"app.kubernetes.io/component": mcoconfig.CardinalityEstimator,
		"app.kubernetes.io/name":      mcoconfig.CardinalityEstimatorServiceName,
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcoconfig.CardinalityEstimatorServiceName,
			Namespace: mcoconfig.GetDefaultNamespace(),
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       cardinalityEstimatorPort,
					TargetPort: intstr.FromInt32(cardinalityEstimatorPort),
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
			log.Info("Creating cardinality estimator Service", "name", svc.Name, "namespace", svc.Namespace)
			return c.Create(ctx, svc)
		}
		return err
	}

	if !maps.Equal(found.Spec.Selector, svc.Spec.Selector) || !reflect.DeepEqual(found.Spec.Ports, svc.Spec.Ports) {
		log.Info("Updating cardinality estimator Service", "name", svc.Name, "namespace", svc.Namespace)
		updated := found.DeepCopy()
		updated.Labels = svc.Labels
		updated.Spec.Selector = svc.Spec.Selector
		updated.Spec.Ports = svc.Spec.Ports
		return c.Update(ctx, updated)
	}

	return nil
}

func reconcileCardinalityEstimatorDeployment(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
	configHash string,
) error {
	deployName := mcoconfig.GetOperandName(mcoconfig.CardinalityEstimator)
	if deployName == "" {
		deployName = mcoconfig.CardinalityEstimatorServiceName
	}

	labels := cardinalityEstimatorLabels(mco)
	selectorLabels := map[string]string{
		"app.kubernetes.io/component": mcoconfig.CardinalityEstimator,
		"app.kubernetes.io/name":      mcoconfig.CardinalityEstimatorServiceName,
	}

	defaultImg := mcoconfig.DefaultImgRepository + "/" + mcoconfig.CardinalityEstimatorImgName + ":" + mcoconfig.DefaultImgTagSuffix
	if replaced, customImg := mcoconfig.ReplaceImage(mco.Annotations, defaultImg, mcoconfig.CardinalityEstimatorKey); replaced {
		defaultImg = customImg
	}

	var resources corev1.ResourceRequirements
	if !mcoconfig.WithoutResourcesRequests(mco.GetAnnotations()) {
		resources = mcoconfig.GetResources(mcoconfig.CardinalityEstimator, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	}

	replicas := mcoconfig.GetReplicas(mcoconfig.CardinalityEstimator, mco.Spec.InstanceSize, mco.Spec.AdvancedConfig)
	receiveWriteURL := fmt.Sprintf(
		"http://%s%s.%s.svc:19291/api/v1/receive",
		mcoconfig.GetOperandNamePrefix(),
		mcoconfig.ThanosReceive,
		mcoconfig.GetDefaultNamespace(),
	)

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
						cardinalityEstimatorConfigHashKey: configHash,
					},
				},
				Spec: corev1.PodSpec{
					NodeSelector: mco.Spec.NodeSelector,
					Tolerations:  mco.Spec.Tolerations,
					ImagePullSecrets: []corev1.LocalObjectReference{
						{Name: mcoconfig.GetImagePullSecret(mco.Spec)},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
					},
					Containers: []corev1.Container{
						{
							Name:            mcoconfig.CardinalityEstimator,
							Image:           defaultImg,
							ImagePullPolicy: mcoconfig.GetImagePullPolicy(mco.Spec),
							Args: []string{
								fmt.Sprintf("--config.file=%s/%s", cardinalityEstimatorMountPath, cardinalityEstimatorConfigFileKey),
								fmt.Sprintf("--web.listen-address=:%d", cardinalityEstimatorPort),
								fmt.Sprintf("--remote-write.url=%s", receiveWriteURL),
								fmt.Sprintf("--remote-write.tenant=%s", mcoconfig.GetTenantUID()),
								"--remote-write.interval=60s",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: cardinalityEstimatorPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: cardinalityEstimatorMountPath,
									ReadOnly:  true,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt32(cardinalityEstimatorPort),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       30,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromInt32(cardinalityEstimatorPort),
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
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: mcoconfig.CardinalityEstimatorConfigMapName,
									},
								},
							},
						},
					},
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
			log.Info("Creating cardinality estimator Deployment", "name", deploy.Name, "namespace", deploy.Namespace)
			return c.Create(ctx, deploy)
		}
		return err
	}

	if !equality.Semantic.DeepEqual(found.Spec, deploy.Spec) || !maps.Equal(found.Labels, deploy.Labels) {
		log.Info("Updating cardinality estimator Deployment", "name", deploy.Name, "namespace", deploy.Namespace)
		updated := found.DeepCopy()
		updated.Labels = deploy.Labels
		updated.Spec = deploy.Spec
		return c.Update(ctx, updated)
	}

	return nil
}
