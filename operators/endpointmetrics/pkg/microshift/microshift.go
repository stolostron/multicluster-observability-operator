// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package microshift

import (
	"context"

	"fmt"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	etcdClientCertSecretName = "etcd-client-cert" //nolint:gosec
)

type Microshift struct {
	// client         client.Client
	addonNamespace string
	hostIP         string
}

func NewMicroshift(addonNs, hostIP string) *Microshift {
	return &Microshift{
		// client:         client,
		addonNamespace: addonNs,
		hostIP:         hostIP,
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

	return nil

}

// renderCronJobExposingMicroshiftSecrets creates a cronjob to expose Microshift's host secrets needed in Microshift itself.
// For example, Microshift clusters run etcd directly on the host. It exposes its metrics via a secured port.
// The job ensures that etcd client key and certificate are exposed as a secret in the addon namespace.
func (m *Microshift) renderCronJobExposingMicroshiftSecrets() ([]*unstructured.Unstructured, error) {
	ret := []*unstructured.Unstructured{}
	jobName := "microshift-secrets-exposer"

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
			Schedule: "0 * * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:    "microshift-certs-updater",
									Image:   "registry.redhat.io/openshift4/ose-cli@sha256:a57eba642e65874fa48738ff5c361e608d4a9b00a47adcf73562925ac52e2204",
									Command: []string{"/bin/sh", "-c"},
									Args: []string{fmt.Sprintf(
										`
                                        oc create secret generic %s --from-file=key=/tmp/etcd-certs/ca.key --from-file=cert=/tmp/etcd-certs/ca.crt --dry-run=client -o yaml | oc apply -f -
                                        `, etcdClientCertSecretName),
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "etcd-certs",
											MountPath: "/tmp/etcd-certs",
											ReadOnly:  true,
										},
									},
									SecurityContext: &corev1.SecurityContext{
										RunAsUser:                new(int64), // 0 for root user
										AllowPrivilegeEscalation: new(bool),
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{"ALL"},
										},
									},
								},
							},
							RestartPolicy:      corev1.RestartPolicyNever,
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
				},
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
				ResourceNames: []string{"hostmount-anyuid"},
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

	// Expose etcd endpoint in the addon namespace
	endpoint := &corev1.Endpoints{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Endpoints",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd",
			Namespace: m.addonNamespace,
			Labels: map[string]string{
				"app": "etcd",
			},
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{
						IP: m.hostIP,
					},
				},
				Ports: []corev1.EndpointPort{
					{
						Name:     "metrics",
						Port:     2381,
						Protocol: corev1.ProtocolTCP,
					},
				},
			},
		},
	}

	unstructuredEndpoint, err := convertToUnstructured(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to convert endpoint to unstructured: %w", err)
	}
	ret = append(ret, unstructuredEndpoint)

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd",
			Namespace: m.addonNamespace,
			Labels: map[string]string{
				"app": "etcd",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Port:       2381,
					TargetPort: intstr.FromInt(2381),
				},
			},
			Selector: map[string]string{
				"app": "etcd",
			},
		},
	}

	unstructuredService, err := convertToUnstructured(service)
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
						CertFile: "/etc/prometheus/secrets/etcd-cert/ca.crt",
						KeyFile:  "/etc/prometheus/secrets/etcd-cert/ca.key",
						CAFile:   "/etc/prometheus/secrets/etcd-cert/ca.crt",
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
