package multiclustermonitoring

import (
	"context"
	"fmt"
	grafanav1alpha1 "github.com/integr8ly/grafana-operator/v3/pkg/apis/integreatly/v1alpha1"
	observatoriumv1alpha1 "github.com/observatorium/configuration/api/v1alpha1"
	monitoringv1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/monitoring/v1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/controller/multiclustermonitoring/util"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/rendering"
	routev1 "github.com/openshift/api/route/v1"
	routev1ClientSet "github.com/openshift/client-go/route/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

var log = logf.Log.WithName("controller_multiclustermonitoring")

const (
	grafanaPartoOfName          = "-grafana"
	observatoriumPartoOfName    = "-observatorium"
	observatoriumAPIGatewayName = "observatorium-api-gateway"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new MultiClusterMonitoring Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMultiClusterMonitoring{client: mgr.GetClient(), apiReader: mgr.GetAPIReader(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("multiclustermonitoring-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource MultiClusterMonitoring
	err = c.Watch(&source.Kind{Type: &monitoringv1.MultiClusterMonitoring{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner MultiClusterMonitoring
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &monitoringv1.MultiClusterMonitoring{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileMultiClusterMonitoring implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileMultiClusterMonitoring{}

// ReconcileMultiClusterMonitoring reconciles a MultiClusterMonitoring object
type ReconcileMultiClusterMonitoring struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client    client.Client
	apiReader client.Reader
	scheme    *runtime.Scheme
}

// Reconcile reads that state of the cluster for a MultiClusterMonitoring object and makes changes based on the state read
// and what is in the MultiClusterMonitoring.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMultiClusterMonitoring) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling MultiClusterMonitoring")

	// Fetch the MultiClusterMonitoring instance
	instance := &monitoringv1.MultiClusterMonitoring{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	//Render the templates with a specified CR
	renderer := rendering.NewRenderer(instance)
	toDeploy, err := renderer.Render(r.client)
	if err != nil {
		reqLogger.Error(err, "Failed to render multiClusterMonitoring templates")
		return reconcile.Result{}, err
	}
	//Deploy the resources
	for _, res := range toDeploy {
		if res.GetNamespace() == instance.Namespace {
			if err := controllerutil.SetControllerReference(instance, res, r.scheme); err != nil {
				reqLogger.Error(err, "Failed to set controller reference")
			}
		}
		if err := deploy(r.client, res); err != nil {
			reqLogger.Error(err, fmt.Sprintf("Failed to deploy %s %s/%s", res.GetKind(), instance.Namespace, res.GetName()))
			return reconcile.Result{}, err
		}
	}

	// create a Observatorium CR
	result, err := r.newObservatoriumCR(instance)
	if result != nil {
		return *result, err
	}

	// create a grafana CR
	result, err = r.newGrafanaCR(instance)
	if result != nil {
		return *result, err
	}

	// expose observatorium api gateway
	result, err = r.newAPIGatewayRoute(instance)
	if result != nil {
		return *result, err
	}
	// have a grafana ingress to integrate with management-ingress

	// generate grafana datasource CR to point to observatorium api gateway
	result, err = r.newGrafanaDataSourceCR(instance)
	if result != nil {
		return *result, err
	}

	// generate/update the configmap cluster-monitoring-config
	result, err = r.newOCPMonitoringCM(instance)
	if result != nil {
		return *result, err
	}

	// Pod already exists - don't requeue
	//reqLogger.Info("Skip reconcile: Pod already exists", "Pod.Namespace", found.Namespace, "Pod.Name", found.Name)
	return reconcile.Result{}, nil
}

func deploy(c client.Client, obj *unstructured.Unstructured) error {
	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(obj.GroupVersionKind())
	err := c.Get(context.TODO(), types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.Create(context.TODO(), obj)
		}
		return err
	}

	if found.GetKind() != "Deployment" {
		return nil
	}

	oldSpec, oldSpecFound := found.Object["spec"]
	newSpec, newSpecFound := obj.Object["spec"]
	if !oldSpecFound || !newSpecFound {
		return nil
	}
	if !reflect.DeepEqual(oldSpec, newSpec) {
		newObj := found.DeepCopy()
		newObj.Object["spec"] = newSpec
		return c.Update(context.TODO(), newObj)
	}
	return nil
}

// newGrafanaCR returns grafana cr defined in MultiClusterMonitoring
func (r *ReconcileMultiClusterMonitoring) newGrafanaCR(cr *monitoringv1.MultiClusterMonitoring) (*reconcile.Result, error) {
	labels := map[string]string{
		"app": cr.Name,
	}
	grafanaCR := &grafanav1alpha1.Grafana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + grafanaPartoOfName,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: cr.Spec.Grafana,
	}
	// Set MultiClusterMonitoring instance as the owner and controller
	if err := controllerutil.SetControllerReference(cr, grafanaCR, r.scheme); err != nil {
		return &reconcile.Result{}, err
	}

	// Check if this Pod already exists
	grafanaCRFound := &grafanav1alpha1.Grafana{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: grafanaCR.Name, Namespace: grafanaCR.Namespace}, grafanaCRFound)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new grafana CR", "grafana.Namespace", grafanaCR.Namespace, "grafana.Name", grafanaCR.Name)
		err = r.client.Create(context.TODO(), grafanaCR)
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

// newObservatoriumCR returns Observatorium cr defined in MultiClusterMonitoring
func (r *ReconcileMultiClusterMonitoring) newObservatoriumCR(cr *monitoringv1.MultiClusterMonitoring) (*reconcile.Result, error) {

	labels := map[string]string{
		"app": cr.Name,
	}
	observatoriumCR := &observatoriumv1alpha1.Observatorium{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + observatoriumPartoOfName,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: cr.Spec.Observatorium,
	}

	// Set MultiClusterMonitoring instance as the owner and controller
	if err := controllerutil.SetControllerReference(cr, observatoriumCR, r.scheme); err != nil {
		return &reconcile.Result{}, err
	}

	// Check if this Pod already exists
	observatoriumCRFound := &observatoriumv1alpha1.Observatorium{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: observatoriumCR.Name, Namespace: observatoriumCR.Namespace}, observatoriumCRFound)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new observatorium CR", "observatorium.Namespace", observatoriumCR.Namespace, "observatorium.Name", observatoriumCR.Name)
		err = r.client.Create(context.TODO(), observatoriumCR)
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

func (r *ReconcileMultiClusterMonitoring) newAPIGatewayRoute(cr *monitoringv1.MultiClusterMonitoring) (*reconcile.Result, error) {
	labelSelector := fmt.Sprintf("app.kubernetes.io/component=%s, app.kubernetes.io/instance=%s", "api-gateway", cr.Name+observatoriumPartoOfName)
	listOptions := metav1.ListOptions{
		LabelSelector: labelSelector,
	}
	kubeClient, err := createKubeClient()
	if err != nil {
		log.Error(err, "Failed to create kube client")
		return &reconcile.Result{}, err
	}

	apiGatewayServices, err := kubeClient.CoreV1().Services(cr.Namespace).List(listOptions)
	if err == nil && len(apiGatewayServices.Items) > 0 {
		apiGateway := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      observatoriumAPIGatewayName,
				Namespace: cr.Namespace,
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
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: apiGateway.Name, Namespace: apiGateway.Namespace}, &routev1.Route{})
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating a new route to expose observatorium api", "apiGateway.Namespace", apiGateway.Namespace, "apiGateway.Name", apiGateway.Name)
			err = r.client.Create(context.TODO(), apiGateway)
			if err != nil {
				return &reconcile.Result{}, err
			}
		}

	} else if err == nil && len(apiGatewayServices.Items) == 0 {
		log.Info("Cannot find the service ", "serviceName", cr.Name+observatoriumPartoOfName+"-"+observatoriumAPIGatewayName)
		return &reconcile.Result{RequeueAfter: time.Second * 10}, nil
	} else {
		return &reconcile.Result{}, err
	}
	return nil, nil
}

func createRoutev1Client() (routev1ClientSet.Interface, error) {
	config, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	routev1Client, err := routev1ClientSet.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return routev1Client, err
}

func (r *ReconcileMultiClusterMonitoring) newGrafanaDataSourceCR(cr *monitoringv1.MultiClusterMonitoring) (*reconcile.Result, error) {
	labels := map[string]string{
		"app": cr.Name,
	}
	grafanaDataSourceCR := &grafanav1alpha1.GrafanaDataSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + grafanaPartoOfName,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: grafanav1alpha1.GrafanaDataSourceSpec{
			Name: observatoriumAPIGatewayName,
			Datasources: []grafanav1alpha1.GrafanaDataSourceFields{
				{
					Name:   "Observatorium",
					Type:   "prometheus",
					Access: "proxy",
					Url:    "http://" + cr.Name + observatoriumPartoOfName + "-observatorium-api-gateway:8080/api/metrics/v1",
				},
			},
		},
	}
	// Set MultiClusterMonitoring instance as the owner and controller
	if err := controllerutil.SetControllerReference(cr, grafanaDataSourceCR, r.scheme); err != nil {
		return &reconcile.Result{}, err
	}

	// Check if this CR already exists
	grafanaDSCRFound := &grafanav1alpha1.GrafanaDataSource{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: grafanaDataSourceCR.Name, Namespace: grafanaDataSourceCR.Namespace}, grafanaDSCRFound)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new grafana CR", "grafanaDataSource.Namespace", grafanaDataSourceCR.Namespace, "grafanaDataSource.Name", grafanaDataSourceCR.Name)
		err = r.client.Create(context.TODO(), grafanaDataSourceCR)
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

func (r *ReconcileMultiClusterMonitoring) newOCPMonitoringCM(cr *monitoringv1.MultiClusterMonitoring) (*reconcile.Result, error) {

	routev1Client, err := createRoutev1Client()
	if err != nil {
		log.Error(err, "Failed to create routev1 client")
		return &reconcile.Result{}, nil
	}

	// Try to get route instance
	obsRoute, err := routev1Client.RouteV1().Routes(cr.Namespace).Get(observatoriumAPIGatewayName, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to get route", observatoriumAPIGatewayName)
		return &reconcile.Result{}, err
	}

	ocpMonitoringCM, err := util.CreateConfigMap(obsRoute.Spec.Host)
	if err != nil {
		log.Error(err, "Failed to create configmap")
		return &reconcile.Result{}, err
	}

	existingCM := &v1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: ocpMonitoringCM.Name, Namespace: ocpMonitoringCM.Namespace}, existingCM)
	if err == nil {
		log.Info("Updating the configmap for cluster monitoring")
		err = util.UpdateConfigMap(existingCM, obsRoute.Spec.Host)
		if err != nil {
			log.Error(err, "Failed to update the configmap")
			return &reconcile.Result{}, err
		}
		err = r.client.Update(context.TODO(), existingCM)
		if err != nil {
			return &reconcile.Result{}, err
		}
	} else if errors.IsNotFound(err) {
		log.Info("Creating the configmap for cluster monitoring")
		ocpMonitoringCM, err := util.CreateConfigMap(obsRoute.Spec.Host)
		if err != nil {
			log.Error(err, "Failed to create configmap")
			return &reconcile.Result{}, err
		}
		err = r.client.Create(context.TODO(), ocpMonitoringCM)
		if err != nil {
			return &reconcile.Result{}, err
		}
	} else {
		return &reconcile.Result{}, err
	}

	return nil, nil
}
