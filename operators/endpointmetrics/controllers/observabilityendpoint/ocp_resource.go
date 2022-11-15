// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package observabilityendpoint

import (
	"context"
	"fmt"
	"os"
	"reflect"

	ocinfrav1 "github.com/openshift/api/config/v1"
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterRoleBindingName = "metrics-collector-view"
	caConfigmapName        = "metrics-collector-serving-certs-ca-bundle"
	etcdServiceMonitor     = "acm-etcd"
	etcdSecName            = "etcd-client-tls"
	kubeApiServiceMonitor  = "acm-kube-apiserver"
	kubeApiSecName         = "metrics-client"
)

var (
	serviceAccountName = os.Getenv("SERVICE_ACCOUNT")
)

func deleteMonitoringClusterRoleBinding(ctx context.Context, client client.Client) error {
	rb := &rbacv1.ClusterRoleBinding{}
	err := client.Get(ctx, types.NamespacedName{Name: clusterRoleBindingName,
		Namespace: ""}, rb)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("clusterrolebinding already deleted")
			return nil
		}
		log.Error(err, "Failed to check the clusterrolebinding")
		return err
	}
	err = client.Delete(ctx, rb)
	if err != nil {
		log.Error(err, "Error deleting clusterrolebinding")
		return err
	}
	log.Info("clusterrolebinding deleted")
	return nil
}

func createMonitoringClusterRoleBinding(ctx context.Context, client client.Client) error {
	rb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleBindingName,
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "cluster-monitoring-view",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		},
	}

	found := &rbacv1.ClusterRoleBinding{}
	err := client.Get(ctx, types.NamespacedName{Name: clusterRoleBindingName,
		Namespace: ""}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			err = client.Create(ctx, rb)
			if err == nil {
				log.Info("clusterrolebinding created")
			} else {
				log.Error(err, "Failed to create the clusterrolebinding")
			}
			return err
		}
		log.Error(err, "Failed to check the clusterrolebinding")
		return err
	}

	if reflect.DeepEqual(rb.RoleRef, found.RoleRef) && reflect.DeepEqual(rb.Subjects, found.Subjects) {
		log.Info("The clusterrolebinding already existed")
	} else {
		rb.ObjectMeta.ResourceVersion = found.ObjectMeta.ResourceVersion
		err = client.Update(ctx, rb)
		if err != nil {
			log.Error(err, "Failed to update the clusterrolebinding")
		}
	}

	return nil
}

func deleteCAConfigmap(ctx context.Context, client client.Client) error {
	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Name: caConfigmapName,
		Namespace: namespace}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("configmap already deleted")
			return nil
		}
		log.Error(err, "Failed to check the configmap")
		return err
	}
	err = client.Delete(ctx, cm)
	if err != nil {
		log.Error(err, "Error deleting configmap")
		return err
	}
	log.Info("configmap deleted")
	return nil
}

func createCAConfigmap(ctx context.Context, client client.Client) error {
	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Name: caConfigmapName,
		Namespace: namespace}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      caConfigmapName,
					Namespace: namespace,
					Annotations: map[string]string{
						ownerLabelKey: ownerLabelValue,
						"service.alpha.openshift.io/inject-cabundle": "true",
					},
				},
				Data: map[string]string{"service-ca.crt": ""},
			}
			err = client.Create(ctx, cm)
			if err == nil {
				log.Info("Configmap created")
			} else {
				log.Error(err, "Failed to create the configmap")
			}
			return err
		} else {
			log.Error(err, "Failed to check the configmap")
			return err
		}
	} else {
		log.Info("The configmap already existed")
	}
	return nil
}

// getClusterID is used to get the cluster uid
func getClusterID(ctx context.Context, c client.Client) (string, error) {
	clusterVersion := &ocinfrav1.ClusterVersion{}
	if err := c.Get(ctx, types.NamespacedName{Name: "version"}, clusterVersion); err != nil {
		log.Error(err, "Failed to get clusterVersion")
		return "", err
	}

	return string(clusterVersion.Spec.ClusterID), nil
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

func createServiceMonitors(ctx context.Context, c client.Client) error {
	hList := &hyperv1.HostedClusterList{}
	err := c.List(context.TODO(), hList, &client.ListOptions{})
	if err != nil {
		log.Error(err, "Failed to list HyperShiftDeployment")
		return err
	}
	for _, cluster := range hList.Items {
		namespace := fmt.Sprintf("clusters-%s", cluster.ObjectMeta.Name)
		id := cluster.Spec.ClusterID
		etcdSM := getEtcdServiceMonitor(namespace, id)
		kubeSM := getKubeServiceMonitor(namespace, id)

		smList := &promv1.ServiceMonitorList{}
		err = c.List(ctx, smList, client.InNamespace(namespace))
		if err != nil {
			log.Error(err, "Failed to list ServiceMonitor", "namespace", namespace)
			return err
		}
		err = createOrUpdateSM(ctx, c, etcdSM, smList)
		if err != nil {
			return err
		}
		err = createOrUpdateSM(ctx, c, kubeSM, smList)
		if err != nil {
			return err
		}
	}
	return nil
}

func createOrUpdateSM(ctx context.Context, c client.Client, updateSm *promv1.ServiceMonitor,
	smList *promv1.ServiceMonitorList) error {
	found := false
	for _, sm := range smList.Items {
		if sm.ObjectMeta.Name == updateSm.ObjectMeta.Name {
			found = true
			if !reflect.DeepEqual(sm.Spec, updateSm.Spec) {
				updateSm.ObjectMeta.ResourceVersion = sm.ObjectMeta.ResourceVersion
				err := c.Update(ctx, updateSm, &client.UpdateOptions{})
				if err != nil {
					log.Error(err, "Failed to update ServiceMonitor", "namespace",
						sm.ObjectMeta.Namespace, "name", sm.ObjectMeta.Name)
					return err
				}
			}
		}
	}
	if !found {
		err := c.Create(ctx, updateSm, &client.CreateOptions{})
		if err != nil {
			log.Error(err, "Failed to create ServiceMonitor", "namespace",
				updateSm.ObjectMeta.Namespace, "name", updateSm.ObjectMeta.Name)
			return err
		}
	}

	return nil
}

func getEtcdServiceMonitor(namespace, id string) *promv1.ServiceMonitor {
	return &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdServiceMonitor,
			Namespace: namespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{
				{
					Scheme:   "https",
					Interval: "15s",
					Port:     "metrics",
					BearerTokenSecret: v1.SecretKeySelector{
						Key: "",
					},
					TLSConfig: &promv1.TLSConfig{
						SafeTLSConfig: promv1.SafeTLSConfig{
							ServerName: "etcd-client",
							CA: promv1.SecretOrConfigMap{
								Secret: &v1.SecretKeySelector{
									LocalObjectReference: v1.LocalObjectReference{
										Name: etcdSecName,
									},
									Key: "etcd-client-ca.crt",
								},
							},
							Cert: promv1.SecretOrConfigMap{
								Secret: &v1.SecretKeySelector{
									LocalObjectReference: v1.LocalObjectReference{
										Name: etcdSecName,
									},
									Key: "etcd-client.crt",
								},
							},
							KeySecret: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: etcdSecName,
								},
								Key: "etcd-client.key",
							},
						},
					},
					MetricRelabelConfigs: []*promv1.RelabelConfig{
						{
							SourceLabels: []string{"__name__"},
							Action:       "keep",
							Regex: "(etcd_server_has_leader|etcd_disk_wal_fsync_duration_seconds_bucket|" +
								"etcd_mvcc_db_total_size_in_bytes|etcd_network_peer_round_trip_time_seconds_bucket|" +
								"etcd_mvcc_db_total_size_in_use_in_bytes|" +
								"etcd_disk_backend_commit_duration_seconds_bucket|" +
								"etcd_server_leader_changes_seen_total)",
						},
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: id,
						},
					},
					RelabelConfigs: []*promv1.RelabelConfig{
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: id,
						},
						{
							TargetLabel: "job",
							Action:      "replace",
							Replacement: "etcd",
						},
					},
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "etcd",
				},
			},
			NamespaceSelector: promv1.NamespaceSelector{
				MatchNames: []string{namespace},
			},
		},
	}
}

func getKubeServiceMonitor(namespace, id string) *promv1.ServiceMonitor {
	return &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeApiServiceMonitor,
			Namespace: namespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			Endpoints: []promv1.Endpoint{
				{
					Scheme:   "https",
					Interval: "15s",
					TargetPort: &intstr.IntOrString{
						StrVal: "6443",
					},
					BearerTokenSecret: v1.SecretKeySelector{
						Key: "",
					},
					TLSConfig: &promv1.TLSConfig{
						SafeTLSConfig: promv1.SafeTLSConfig{
							ServerName: "kube-apiserver",
							CA: promv1.SecretOrConfigMap{
								Secret: &v1.SecretKeySelector{
									LocalObjectReference: v1.LocalObjectReference{
										Name: kubeApiSecName,
									},
									Key: "ca.crt",
								},
							},
							Cert: promv1.SecretOrConfigMap{
								Secret: &v1.SecretKeySelector{
									LocalObjectReference: v1.LocalObjectReference{
										Name: kubeApiSecName,
									},
									Key: "tls.crt",
								},
							},
							KeySecret: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: kubeApiSecName,
								},
								Key: "tls.key",
							},
						},
					},
					MetricRelabelConfigs: []*promv1.RelabelConfig{
						{
							SourceLabels: []string{"__name__"},
							Action:       "keep",
							Regex: "(up|apiserver_request_duration_seconds_bucket|apiserver_storage_objects|" +
								"apiserver_request_total|apiserver_current_inflight_requests)",
						},
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: id,
						},
					},
					RelabelConfigs: []*promv1.RelabelConfig{
						{
							TargetLabel: "_id",
							Action:      "replace",
							Replacement: id,
						},
						{
							TargetLabel: "job",
							Action:      "replace",
							Replacement: "apiserver",
						},
					},
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "kube-apiserver",
					"hypershift.openshift.io/control-plane-component": "kube-apiserver",
				},
			},
			NamespaceSelector: promv1.NamespaceSelector{
				MatchNames: []string{namespace},
			},
		},
	}
}

func deleteServiceMonitors(ctx context.Context, c client.Client) error {
	hList := &hyperv1.HostedClusterList{}
	err := c.List(context.TODO(), hList, &client.ListOptions{})
	if err != nil {
		log.Error(err, "Failed to list HyperShiftDeployment")
		return err
	}
	for _, cluster := range hList.Items {
		namespace := fmt.Sprintf("clusters-%s", cluster.ObjectMeta.Name)
		err = deleteServiceMonitor(ctx, c, etcdServiceMonitor, namespace)
		if err != nil {
			return err
		}
		err = deleteServiceMonitor(ctx, c, kubeApiServiceMonitor, namespace)
		if err != nil {
			return err
		}
	}
	return nil
}

func deleteServiceMonitor(ctx context.Context, c client.Client, name, namespace string) error {
	sm := &promv1.ServiceMonitor{}
	err := c.Get(ctx, types.NamespacedName{Name: name,
		Namespace: namespace}, sm)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("ServiceMonitor already deleted", "namespace", namespace, "name", name)
			return nil
		}
		log.Error(err, "Failed to check the ServiceMonitor", "namespace", namespace, "name", name)
		return err
	}
	err = c.Delete(ctx, sm)
	if err != nil {
		log.Error(err, "Error deleting ServiceMonitor", namespace, "name", name)
		return err
	}
	log.Info("ServiceMonitor deleted", namespace, "name", name)
	return nil
}
