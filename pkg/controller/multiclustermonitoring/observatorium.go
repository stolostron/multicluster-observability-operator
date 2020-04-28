package multiclustermonitoring

import (
	"context"
	"fmt"
	"time"

	observatoriumv1alpha1 "github.com/observatorium/configuration/api/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	monitoringv1alpha1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1alpha1"
)

const (
	observatoriumPartoOfName    = "-observatorium"
	observatoriumAPIGatewayName = "observatorium-api-gateway"
)

// GenerateObservatoriumCR returns Observatorium cr defined in MultiClusterMonitoring
func GenerateObservatoriumCR(client client.Client, scheme *runtime.Scheme, monitoring *monitoringv1alpha1.MultiClusterMonitoring) (*reconcile.Result, error) {

	labels := map[string]string{
		"app": monitoring.Name,
	}
	observatoriumCR := &observatoriumv1alpha1.Observatorium{
		ObjectMeta: metav1.ObjectMeta{
			Name:      monitoring.Name + observatoriumPartoOfName,
			Namespace: monitoring.Namespace,
			Labels:    labels,
		},
		Spec: monitoring.Spec.Observatorium,
	}

	// Set MultiClusterMonitoring instance as the owner and controller
	if err := controllerutil.SetControllerReference(monitoring, observatoriumCR, scheme); err != nil {
		return &reconcile.Result{}, err
	}

	// Check if this Observatorium CR already exists
	observatoriumCRFound := &observatoriumv1alpha1.Observatorium{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: observatoriumCR.Name, Namespace: observatoriumCR.Namespace}, observatoriumCRFound)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new observatorium CR", "observatorium.Namespace", observatoriumCR.Namespace, "observatorium.Name", observatoriumCR.Name)
		err = client.Create(context.TODO(), observatoriumCR)
		if err != nil {
			return &reconcile.Result{}, err
		}

		// Pod created successfully - don't requeue
		return nil, nil
	} else if err != nil {
		return &reconcile.Result{}, err
	}

	return nil, nil
}

func createKubeClient() (kubernetes.Interface, error) {
	config, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return kubeClient, err
}

func GenerateAPIGatewayRoute(client client.Client, scheme *runtime.Scheme, monitoring *monitoringv1alpha1.MultiClusterMonitoring) (*reconcile.Result, error) {
	labelSelector := fmt.Sprintf("app.kubernetes.io/component=%s, app.kubernetes.io/instance=%s", "api-gateway", monitoring.Name+observatoriumPartoOfName)
	listOptions := metav1.ListOptions{
		LabelSelector: labelSelector,
	}
	kubeClient, err := createKubeClient()
	if err != nil {
		log.Error(err, "Failed to create kube client")
		return &reconcile.Result{}, err
	}

	apiGatewayServices, err := kubeClient.CoreV1().Services(monitoring.Namespace).List(listOptions)
	if err == nil && len(apiGatewayServices.Items) > 0 {
		apiGateway := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      observatoriumAPIGatewayName,
				Namespace: monitoring.Namespace,
			},
			Spec: routev1.RouteSpec{
				Port: &routev1.RoutePort{
					TargetPort: intstr.FromString("http"),
				},
				To: routev1.RouteTargetReference{
					Kind: "Service",
					Name: apiGatewayServices.Items[0].GetName(),
				},
			},
		}
		err = client.Get(context.TODO(), types.NamespacedName{Name: apiGateway.Name, Namespace: apiGateway.Namespace}, &routev1.Route{})
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating a new route to expose observatorium api", "apiGateway.Namespace", apiGateway.Namespace, "apiGateway.Name", apiGateway.Name)
			err = client.Create(context.TODO(), apiGateway)
			if err != nil {
				return &reconcile.Result{}, err
			}
		}

	} else if err == nil && len(apiGatewayServices.Items) == 0 {
		log.Info("Cannot find the service ", "serviceName", monitoring.Name+observatoriumPartoOfName+"-"+observatoriumAPIGatewayName)
		return &reconcile.Result{RequeueAfter: time.Second * 10}, nil
	} else {
		return &reconcile.Result{}, err
	}
	return nil, nil
}
