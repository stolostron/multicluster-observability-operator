// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package microshift

import (
	"context"
	"strings"

	"fmt"

	"github.com/go-logr/logr"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	promcommon "github.com/prometheus/common/config"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	etcdClientCertSecretName  = "etcd-client-cert" //nolint:gosec
	prometheusScrapeCfgSecret = "prometheus-scrape-targets"
	scrapeConfigKey           = "scrape-targets.yaml"
)

type Microshift struct {
	client         client.Client
	addonNamespace string
	logger         logr.Logger
}

func NewMicroshift(c client.Client, addonNs string, logger logr.Logger) *Microshift {
	return &Microshift{
		addonNamespace: addonNs,
		client:         c,
		logger:         logger.WithName("microshift"),
	}
}

// Render renders the resources for the microshift cluster
// If the cluster is not a microshift cluster, it modifies the resources
// to adapt to the microshift cluster
func (m *Microshift) Render(ctx context.Context, resources []*unstructured.Unstructured) ([]*unstructured.Unstructured, error) {
	jobRes, err := m.renderCronJobExposingMicroshiftSecrets()
	if err != nil {
		return nil, fmt.Errorf("failed to render cronjob for secrets: %w", err)
	}
	resources = append(resources, jobRes...)

	etcdRes, err := m.renderEtcdResources()
	if err != nil {
		return nil, fmt.Errorf("failed to render etcd resources: %w", err)
	}
	resources = append(resources, etcdRes...)

	if err := m.renderPrometheus(resources); err != nil {
		return nil, fmt.Errorf("failed to render prometheus: %w", err)
	}

	if err := m.renderScrapeConfig(resources); err != nil {
		return nil, fmt.Errorf("failed to render scrape config: %w", err)
	}

	return resources, nil
}

// renderPrometheus modifies the prometheus resource to adapt to the microshift cluster
// It adds the etcd client key and certificate secret to the prometheus pod
func (m *Microshift) renderPrometheus(res []*unstructured.Unstructured) error {
	promRes, err := getResource(res, "Prometheus", "k8s")
	if err != nil {
		return fmt.Errorf("failed to get prometheus resource: %w", err)
	}

	prom := &promv1.Prometheus{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(promRes.Object, prom); err != nil {
		return fmt.Errorf("failed to convert unstructured object to prometheus object: %w", err)
	}

	prom.Spec.Secrets = append(prom.Spec.Secrets, etcdClientCertSecretName)
	prom.Spec.HostNetwork = true

	// check that additional scrape config is as expected
	if prom.Spec.AdditionalScrapeConfigs == nil || prom.Spec.AdditionalScrapeConfigs.LocalObjectReference.Name != prometheusScrapeCfgSecret {
		return fmt.Errorf(fmt.Sprintf("additional scrape config is not as expected, want %s, got %s", prometheusScrapeCfgSecret, prom.Spec.AdditionalScrapeConfigs.LocalObjectReference.Name))
	}

	promRes.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(prom)
	if err != nil {
		return fmt.Errorf("failed to convert prometheus object to unstructured object: %w", err)
	}

	return nil

}

func (m *Microshift) renderScrapeConfig(res []*unstructured.Unstructured) error {
	secret, err := getResource(res, "Secret", prometheusScrapeCfgSecret)
	if err != nil {
		return fmt.Errorf("failed to get prometheus scrape secret resource: %w", err)
	}

	scrapeSecret := &corev1.Secret{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(secret.Object, scrapeSecret); err != nil {
		return fmt.Errorf("failed to convert unstructured object to secret object: %w", err)
	}

	etcdScrapeCfg := ScrapeConfig{}
	etcdScrapeCfg.JobName = "etcd"
	etcdScrapeCfg.Scheme = "https"
	etcdScrapeCfg.HTTPClientConfig = promcommon.HTTPClientConfig{
		TLSConfig: promcommon.TLSConfig{
			CertFile: fmt.Sprintf("/etc/prometheus/secrets/%s/ca.crt", etcdClientCertSecretName),
			KeyFile:  fmt.Sprintf("/etc/prometheus/secrets/%s/ca.key", etcdClientCertSecretName),
			CAFile:   fmt.Sprintf("/etc/prometheus/secrets/%s/ca.crt", etcdClientCertSecretName),
		},
	}
	etcdScrapeCfg.StaticConfigs = []StaticConfig{
		{
			Targets: []string{"localhost:2381"},
		},
	}
	newScrapeCfgs := &ScrapeConfigs{
		ScrapeConfigs: []ScrapeConfig{etcdScrapeCfg},
	}

	// Append additional scrape config for etcd on the host
	// We don't unmarshal the existing scrape config to avoid adding default values
	// when marshaling the new scrape config. Instead, we append the new scrape config
	// to the existing scrape config.
	var ret strings.Builder
	ret.WriteString(strings.TrimSpace(scrapeSecret.StringData[scrapeConfigKey]))

	scrapeCfgsYaml, err := newScrapeCfgs.MarshalYAML()
	if err != nil {
		return fmt.Errorf("failed to marshal scrape config: %w", err)
	}

	ret.WriteString("\n")
	ret.WriteString(strings.TrimSpace(string(scrapeCfgsYaml)))
	newScrapeConfigYaml := ret.String()

	scrapeSecret.StringData[scrapeConfigKey] = newScrapeConfigYaml

	secret.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(scrapeSecret)
	if err != nil {
		return fmt.Errorf("failed to convert secret object to unstructured object: %w", err)
	}

	return nil
}

// renderCronJobExposingMicroshiftSecrets creates a cronjob to expose Microshift's host secrets needed in Microshift itself.
// For example, Microshift clusters run etcd directly on the host. It exposes its metrics via a secured port.
// The job ensures that etcd client key and certificate are exposed as a secret in the addon namespace.
func (m *Microshift) renderCronJobExposingMicroshiftSecrets() ([]*unstructured.Unstructured, error) {
	ret := []*unstructured.Unstructured{}
	jobName := "microshift-secrets-exposer"
	completions := int32(1)
	deadline := int64(60)
	ttl := int32(60)
	trueVal := true

	jobSpec := batchv1.JobSpec{
		Completions:             &completions,
		ActiveDeadlineSeconds:   &deadline,
		TTLSecondsAfterFinished: &ttl,
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers: []corev1.Container{
					{
						Name:    "microshift-certs-updater",
						Image:   "registry.redhat.io/openshift4/ose-cli@sha256:a57eba642e65874fa48738ff5c361e608d4a9b00a47adcf73562925ac52e2204",
						Command: []string{"/bin/sh", "-c"},
						Args: []string{fmt.Sprintf(
							`
							oc create secret generic %s --from-file=ca.key=/tmp/etcd-certs/ca.key --from-file=ca.crt=/tmp/etcd-certs/ca.crt --dry-run=client -o yaml -n %s | oc apply -f -
							`, etcdClientCertSecretName, m.addonNamespace),
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "etcd-certs",
								MountPath: "/tmp/etcd-certs",
								ReadOnly:  true,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							ReadOnlyRootFilesystem: &trueVal,
							Privileged:             &trueVal,
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("50m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("20m"),
								corev1.ResourceMemory: resource.MustParse("50Mi"),
							},
						},
					},
				},
				ServiceAccountName: jobName,
				Volumes: []corev1.Volume{
					{
						Name: "etcd-certs",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/var/lib/microshift/certs/etcd-signer/",
								Type: newHostPathType(corev1.HostPathDirectory),
							},
						},
					},
				},
			},
		},
	}

	// If the secret does not exist, start a job to create it, then rely on the cronjob to update it
	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "init-" + jobName,
			Namespace: m.addonNamespace,
		},
		Spec: jobSpec,
	}
	foundJob := &batchv1.Job{}
	err := m.client.Get(context.Background(), types.NamespacedName{
		Name:      job.Name,
		Namespace: job.Namespace,
	}, foundJob)
	if err != nil {
		if errors.IsNotFound(err) {
			err = m.client.Create(context.Background(), job)
			if err != nil && !errors.IsAlreadyExists(err) {
				// If the job already exists, it means it is already running, so we can ignore the error
				return nil, fmt.Errorf("failed to create the init job: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to get secret %s/%s: %w", m.addonNamespace, etcdClientCertSecretName, err)
		}
	}

	// Create a cronjob to update the etcd client key and certificate secret
	// every hour
	cronJob := &batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "CronJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: m.addonNamespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          "0/20 * * * *",
			ConcurrencyPolicy: batchv1.ReplaceConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: jobSpec,
			},
		},
	}

	unstructuredCronJob, err := convertToUnstructured(cronJob)
	if err != nil {
		return nil, fmt.Errorf("failed to convert cronjob to unstructured: %w", err)
	}
	ret = append(ret, unstructuredCronJob)

	// Add service account to the cronjob
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: m.addonNamespace,
		},
	}

	unstructuredSA, err := convertToUnstructured(sa)
	if err != nil {
		return nil, fmt.Errorf("failed to convert service account to unstructured: %w", err)
	}
	ret = append(ret, unstructuredSA)

	// Add permissions to the service account to update the secret and run as root
	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: m.addonNamespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"create", "update", "get", "list", "patch"},
			},
		},
	}

	unstructuredRole, err := convertToUnstructured(role)
	if err != nil {
		return nil, fmt.Errorf("failed to convert role to unstructured: %w", err)
	}
	ret = append(ret, unstructuredRole)

	roleBinding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: m.addonNamespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      jobName,
				Namespace: m.addonNamespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     jobName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	unstructuredRoleBinding, err := convertToUnstructured(roleBinding)
	if err != nil {
		return nil, fmt.Errorf("failed to convert role binding to unstructured: %w", err)
	}
	ret = append(ret, unstructuredRoleBinding)

	// Add cluster role for hostmount and anyuid permissions- apiGroups:
	clusterRole := &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{"security.openshift.io"},
				Resources:     []string{"securitycontextconstraints"},
				ResourceNames: []string{"privileged"},
				Verbs:         []string{"use"},
			},
		},
	}

	unstructuredClusterRole, err := convertToUnstructured(clusterRole)
	if err != nil {
		return nil, fmt.Errorf("failed to convert cluster role to unstructured: %w", err)
	}
	ret = append(ret, unstructuredClusterRole)

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      jobName,
				Namespace: m.addonNamespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     jobName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	unstructuredClusterRoleBinding, err := convertToUnstructured(clusterRoleBinding)
	if err != nil {
		return nil, fmt.Errorf("failed to convert cluster role binding to unstructured: %w", err)
	}
	ret = append(ret, unstructuredClusterRoleBinding)

	return ret, nil
}

// renderServiceMonitors modifies or creates the service monitors to adapt to the microshift cluster
func (m *Microshift) renderEtcdResources() ([]*unstructured.Unstructured, error) {
	ret := []*unstructured.Unstructured{}

	// create secret containing scrape config for etcd running on the host
	scrapeCfg := ScrapeConfig{}
	scrapeCfg.JobName = "etcd"
	scrapeCfg.Scheme = "https"
	scrapeCfg.HTTPClientConfig = promcommon.HTTPClientConfig{
		TLSConfig: promcommon.TLSConfig{
			CertFile: fmt.Sprintf("/etc/prometheus/secrets/%s/ca.crt", etcdClientCertSecretName),
			KeyFile:  fmt.Sprintf("/etc/prometheus/secrets/%s/ca.key", etcdClientCertSecretName),
			CAFile:   fmt.Sprintf("/etc/prometheus/secrets/%s/ca.crt", etcdClientCertSecretName),
		},
	}
	scrapeCfg.StaticConfigs = []StaticConfig{
		{
			Targets: []string{"localhost:2381"},
		},
	}
	scapeCfgs := ScrapeConfigs{
		ScrapeConfigs: []ScrapeConfig{scrapeCfg},
	}

	scrapeCfgsYaml, err := scapeCfgs.MarshalYAML()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal scrape config: %w", err)
	}

	scrapeSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      prometheusScrapeCfgSecret,
			Namespace: m.addonNamespace,
		},
		Data: map[string][]byte{
			scrapeConfigKey: scrapeCfgsYaml,
		},
	}
	unstructuredService, err := convertToUnstructured(scrapeSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to convert service to unstructured: %w", err)
	}
	ret = append(ret, unstructuredService)

	// render service monitor for etcd
	smon := &promv1.ServiceMonitor{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "monitoring.coreos.com/v1",
			Kind:       "ServiceMonitor",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd",
			Namespace: m.addonNamespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{
				{
					Scheme:   "https",
					Path:     "/metrics",
					Interval: "15s",
					// Use secret etcd-cert to scrape etcd metrics
					TLSConfig: &promv1.TLSConfig{
						// /etc/prometheus/secrets/etcd-client-cert
						CertFile: fmt.Sprintf("/etc/prometheus/secrets/%s/ca.crt", etcdClientCertSecretName),
						KeyFile:  fmt.Sprintf("/etc/prometheus/secrets/%s/ca.key", etcdClientCertSecretName),
						CAFile:   fmt.Sprintf("/etc/prometheus/secrets/%s/ca.crt", etcdClientCertSecretName),
					},
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "etcd",
				},
			},
			NamespaceSelector: promv1.NamespaceSelector{
				MatchNames: []string{m.addonNamespace},
			},
		},
	}

	unstructuredSMon, err := convertToUnstructured(smon)
	if err != nil {
		return nil, fmt.Errorf("failed to convert service monitor to unstructured: %w", err)
	}
	ret = append(ret, unstructuredSMon)

	return ret, nil
}

// IsMicroshiftCluster checks if the cluster is a microshift cluster.
// It verifies the existence of the configmap microshift-version in namespace kube-public.
// If the configmap exists, it returns the version of the microshift cluster.
// If the configmap does not exist, it returns an empty string.
func IsMicroshiftCluster(ctx context.Context, client client.Client) (string, error) {
	res := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      "microshift-version",
		Namespace: "kube-public",
	}, res)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	return res.Data["version"], nil
}

func getResource(res []*unstructured.Unstructured, kind, name string) (*unstructured.Unstructured, error) {
	for _, r := range res {
		if r.GetKind() == kind && r.GetName() == name {
			return r, nil
		}
	}
	return nil, errors.NewNotFound(schema.GroupResource{
		Group:    "",
		Resource: kind,
	}, name)
}

func convertToUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	return &unstructured.Unstructured{Object: unstructuredObj}, nil
}

func newHostPathType(pathType corev1.HostPathType) *corev1.HostPathType {
	return &pathType
}
