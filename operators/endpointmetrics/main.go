// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"github.com/IBM/controller-filtered-cache/filteredcache"
	ocinfrav1 "github.com/openshift/api/config/v1"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	obsepctl "github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	statusctl "github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/status"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/openshift"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/version"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	oav1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/status"
	operatorsutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	scheme   = k8sruntime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(oav1beta1.AddToScheme(scheme))
	utilruntime.Must(oav1beta2.AddToScheme(scheme))
	utilruntime.Must(ocinfrav1.AddToScheme(scheme))
	utilruntime.Must(prometheusv1.AddToScheme(scheme))
	utilruntime.Must(hyperv1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func printVersion() {
	setupLog.Info(fmt.Sprintf("Operator Version: %s", version.Version))
	setupLog.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	setupLog.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

func main() {
	printVersion()
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8383", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	klog.InitFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(klog.NewKlogr())

	namespaceSelector := fmt.Sprintf("metadata.namespace==%s", os.Getenv("WATCH_NAMESPACE"))
	gvkLabelMap := map[schema.GroupVersionKind][]filteredcache.Selector{
		v1.SchemeGroupVersion.WithKind("Secret"): {
			{FieldSelector: namespaceSelector},
		},
		v1.SchemeGroupVersion.WithKind("ConfigMap"): {
			{FieldSelector: namespaceSelector},
			{FieldSelector: fmt.Sprintf("metadata.name==%s,metadata.namespace!=%s",
				operatorconfig.AllowlistCustomConfigMapName, "open-cluster-management-observability")},
			{FieldSelector: fmt.Sprintf("metadata.name==%s,metadata.namespace==%s",
				operatorconfig.OCPClusterMonitoringConfigMapName, operatorconfig.OCPClusterMonitoringNamespace)},
		},
		appsv1.SchemeGroupVersion.WithKind("Deployment"): {
			{FieldSelector: namespaceSelector},
		},
		oav1beta1.GroupVersion.WithKind("ObservabilityAddon"): {
			{FieldSelector: namespaceSelector},
		},
	}

	// Only watch MCO CRs in the hub cluster to avoid noisy log messages
	if os.Getenv("HUB_ENDPOINT_OPERATOR") == "true" {
		gvkLabelMap[oav1beta2.GroupVersion.WithKind("MultiClusterObservability")] = []filteredcache.Selector{
			{FieldSelector: "metadata.name!=null"},
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "7c30ca38.open-cluster-management.io",
		NewCache:               filteredcache.NewEnhancedFilteredCacheBuilder(gvkLabelMap),
		WebhookServer:          ctrlwebhook.NewServer(ctrlwebhook.Options{Port: 9443}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	hubClientWithReload, err := util.NewReloadableHubClient(os.Getenv("HUB_KUBECONFIG"), mgr.GetScheme())
	if err != nil {
		setupLog.Error(err, "Failed to create the hub client")
		os.Exit(1)
	}

	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = os.Getenv("WATCH_NAMESPACE")
	}

	var installPrometheus bool
	if envVal := os.Getenv(operatorconfig.InstallPrometheus); envVal != "" {
		installPrometheus, err = strconv.ParseBool(envVal)
		if err != nil {
			setupLog.Error(err, "Failed to parse the value of the environment variable", "variable", operatorconfig.InstallPrometheus)
		}
	}

	obsAddonCtrlLogger := ctrl.Log.WithName("controllers").WithName("ObservabilityAddon")
	obsaddonreconciler := &obsepctl.ObservabilityAddonReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		HubClient:             hubClientWithReload,
		HubNamespace:          os.Getenv("HUB_NAMESPACE"),
		Namespace:             namespace,
		ServiceAccountName:    os.Getenv("SERVICE_ACCOUNT"),
		IsHubMetricsCollector: os.Getenv("HUB_ENDPOINT_OPERATOR") == "true",
		InstallPrometheus:     installPrometheus,
		Logger:                obsAddonCtrlLogger,
	}
	if !obsaddonreconciler.IsHubMetricsCollector {
		// Only add on spokes as there is no addon on the hub and status update would fail
		statusReporter := status.NewStatus(mgr.GetClient(), operatorconfig.ObservabilityAddonName, namespace, obsAddonCtrlLogger)
		obsaddonreconciler.CmoReconcilesDetector = openshift.NewCmoConfigChangesWatcher(mgr.GetClient(), obsAddonCtrlLogger.WithName("cmoWatcher"), statusReporter, 5, 5*time.Minute, 0.6)
	}
	if err = (obsaddonreconciler).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ObservabilityAddon")
		os.Exit(1)
	}

	if err = (&statusctl.StatusReconciler{
		Client:       mgr.GetClient(),
		HubClient:    hubClientWithReload,
		Namespace:    namespace,
		HubNamespace: os.Getenv("HUB_NAMESPACE"),
		ObsAddonName: "observability-addon",
		Logger:       ctrl.Log.WithName("controllers").WithName("Status"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Status")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := operatorsutil.RegisterDebugEndpoint(mgr.AddMetricsServerExtraHandler); err != nil {
		setupLog.Error(err, "unable to set up debug handler")
		os.Exit(1)
	}

	// start lease
	util.StartLease()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
