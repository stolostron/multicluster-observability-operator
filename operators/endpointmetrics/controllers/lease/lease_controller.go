package lease

import (
	"context"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const (
	leaseUpdateJitterFactor     = 0.25
	defaultLeaseDurationSeconds = 60
)

// LeaseUpdater is to update lease with certain period
type LeaseUpdater interface {
	// Start starts a goroutine to update lease
	Start(ctx context.Context)

	// WithHubLeaseConfig sets the lease config on hub cluster. It allows LeaseUpdater to create/update
	// addon lease on hub cluster when resource 'Lease' is not available on managed cluster.
	WithHubLeaseConfig(hubKubeConfigPath string, clusterName string) LeaseUpdater
}

// leaseUpdater update lease of with given name and namespace
type leaseUpdater struct {
	kubeClient           kubernetes.Interface
	leaseName            string
	leaseNamespace       string
	leaseDurationSeconds int32
	clusterName          string
	hubKubeConfigPath    string
	hubKubeClient        kubernetes.Interface
	healthCheckFuncs     []func() bool
}

func NewLeaseUpdater(
	kubeClient kubernetes.Interface,
	leaseName, leaseNamespace string,
	healthCheckFuncs ...func() bool,
) LeaseUpdater {
	return &leaseUpdater{
		kubeClient:           kubeClient,
		leaseName:            leaseName,
		leaseNamespace:       leaseNamespace,
		leaseDurationSeconds: defaultLeaseDurationSeconds,
		healthCheckFuncs:     healthCheckFuncs,
	}
}

func (r *leaseUpdater) Start(ctx context.Context) {
	wait.JitterUntilWithContext(context.TODO(), r.reconcile, time.Duration(r.leaseDurationSeconds)*time.Second, leaseUpdateJitterFactor, true)
}

func (r *leaseUpdater) WithHubLeaseConfig(hubKubeConfigPath string, clusterName string) LeaseUpdater {
	r.hubKubeConfigPath = hubKubeConfigPath
	r.clusterName = clusterName
	err := r.setHubKubeClient()
	if err != nil {
		klog.Errorf("Failed to build hub kube client %v", err)
	}

	return r
}

func (r *leaseUpdater) setHubKubeClient() error {
	hubConfig, err := clientcmd.BuildConfigFromFlags("", r.hubKubeConfigPath)
	if err != nil {
		klog.Fatalf("Failed to load the hub config from path %v", err)
		panic(err.Error())
	}

	hubClient, err := kubernetes.NewForConfig(hubConfig)
	if err != nil {
		klog.Errorf("Failed to build hub kube client %v", err)
	} else {
		r.hubKubeClient = hubClient
	}

	return err
}

func (r *leaseUpdater) updateLease(ctx context.Context, namespace string, client kubernetes.Interface) error {
	lease, err := client.CoordinationV1().Leases(namespace).Get(ctx, r.leaseName, metav1.GetOptions{})
	switch {
	case errors.IsNotFound(err):
		//create lease
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.leaseName,
				Namespace: namespace,
			},
			Spec: coordinationv1.LeaseSpec{
				LeaseDurationSeconds: &r.leaseDurationSeconds,
				RenewTime: &metav1.MicroTime{
					Time: time.Now(),
				},
			},
		}
		if _, err := client.CoordinationV1().Leases(namespace).Create(ctx, lease, metav1.CreateOptions{}); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		//update lease
		lease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
		if _, err = client.CoordinationV1().Leases(namespace).Update(context.TODO(), lease, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}

	return nil
}

func (r *leaseUpdater) reconcile(ctx context.Context) {
	for _, f := range r.healthCheckFuncs {
		if !f() {
			// IF a healthy check fails, do not update lease.
			return
		}
	}
	// Update lease on managed cluster at first, if it returns invalid, it means lease is not supported yet
	// and fallback to use hub lease.
	err := r.updateLease(ctx, r.leaseNamespace, r.kubeClient)
	if err != nil {
		klog.Errorf("Failed to update lease %s/%s: %v on managed cluster", r.leaseName, r.leaseNamespace, err)
	}

	if errors.IsNotFound(err) && r.hubKubeClient != nil {
		klog.V(2).Infof("Trying to update lease on the hub")

		if err := r.updateLease(ctx, r.clusterName, r.hubKubeClient); err != nil {
			klog.Errorf("Failed to update lease %s/%s: %v on hub", r.clusterName, r.leaseNamespace, err)
			// refresh hub client to ensure the error is not caused by stale config
			r.setHubKubeClient()
		}
	}
}

// CheckAddonPodFunc checks whether the agent pod is running
func CheckAddonPodFunc(podGetter corev1client.PodsGetter, namespace, labelSelector string) func() bool {
	return func() bool {
		pods, err := podGetter.Pods(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			klog.Errorf("Failed to get pods in namespace %s with label selector %s: %v", namespace, labelSelector, err)
			return false
		}

		// If one of the pods is running, we think the agent is serving.
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				return true
			}
		}

		return false
	}
}
