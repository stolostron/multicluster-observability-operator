// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
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
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/mcoa"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
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

var (
	mcoaRunner    = runMCOA
	legacyRunner  = runLegacy
	cleanupRunner = runCleanup
)

func main() {
	printVersion()
	execute(os.Args)
}

func execute(args []string) {
	cmd := "legacy"
	if len(args) > 1 {
		cmd = args[1]
	}

	switch cmd {
	case "mcoa":
		// len(args) >= 2 is guaranteed since args[1] matches "mcoa"
		mcoaRunner(args[2:])
	case "legacy":
		// Guard against len(args) == 1 (no arguments) defaulting to legacy mode
		subArgs := []string{}
		if len(args) > 2 {
			subArgs = args[2:]
		}
		legacyRunner(subArgs)
	case "cleanup":
		// len(args) >= 2 is guaranteed since args[1] matches "cleanup"
		cleanupRunner(args[2:])
	default:
		// default to legacy for backward compatibility if argument is just a flag
		// len(args) >= 2 is guaranteed since len(args) > 1 and it did not match subcommands above
		legacyRunner(args[1:])
	}
}

func runCleanup(args []string) {
	setupLog.Info("Starting MCOA cleanup")
	fs := flag.NewFlagSet("cleanup", flag.ExitOnError)
	var hubID string

	fs.StringVar(&hubID, "hub-id", "", "The ID of the Hub cluster to clean up.")
	klog.InitFlags(fs)
	_ = fs.Parse(args)

	if hubID == "" {
		setupLog.Error(fmt.Errorf("hub-id flag not set"), "unable to perform cleanup")
		os.Exit(1)
	}

	cfg := ctrl.GetConfigOrDie()
	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create client for cleanup")
		os.Exit(1)
	}

	hubInfo := &operatorconfig.HubInfo{
		HubClusterID: hubID,
	}

	ctx := context.Background()
	var errs []error

	setupLog.Info("Reverting Platform monitoring configuration")
	if err := obsepctl.RevertClusterMonitoringConfig(ctx, cl, hubInfo); err != nil {
		setupLog.Error(err, "failed to revert platform monitoring config")
		errs = append(errs, err)
	}

	setupLog.Info("Reverting User Workload monitoring configuration")
	if err := obsepctl.RevertUserWorkloadMonitoringConfig(ctx, cl, hubInfo); err != nil {
		setupLog.Error(err, "failed to revert user workload monitoring config")
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		setupLog.Error(fmt.Errorf("cleanup failed with %d errors", len(errs)), "best-effort cleanup incomplete")
		os.Exit(1)
	}

	setupLog.Info("Cleanup completed successfully")
}

func runMCOA(args []string) {
	setupLog.Info("Starting MCOA mode")
	fs := flag.NewFlagSet("mcoa", flag.ExitOnError)
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var hubAmURL string
	var hubID string
	var clusterID string
	var namespace string
	var hubAmCASecret string
	var hubAmCertSecret string
	var hubAmAccessorSecret string

	fs.StringVar(&metricsAddr, "metrics-bind-address", ":8383", "The address the metric endpoint binds to.")
	fs.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&hubAmURL, "hub-alertmanager-url", "", "The URL of the Hub's Alertmanager.")
	fs.StringVar(&hubID, "hub-id", "", "The ID of the Hub cluster.")
	fs.StringVar(&clusterID, "cluster-id", "", "The ID of the managed cluster.")
	fs.StringVar(&namespace, "namespace", "", "The namespace the operator is running in.")
	fs.StringVar(&hubAmCASecret, "hub-alertmanager-ca-secret", "", "The name of the CA secret for the Hub's Alertmanager.")
	fs.StringVar(&hubAmCertSecret, "hub-alertmanager-cert-secret", "", "The name of the TLS cert/key secret for the Hub's Alertmanager.")
	fs.StringVar(&hubAmAccessorSecret, "hub-alertmanager-accessor-secret", "", "The name of the accessor token secret for the Hub's Alertmanager.")

	klog.InitFlags(fs)
	_ = fs.Parse(args)

	ctrl.SetLogger(klog.NewKlogr())

	if namespace == "" {
		setupLog.Error(fmt.Errorf("namespace flag not set"), "unable to start manager")
		os.Exit(1)
	}

	if hubID == "" {
		setupLog.Error(fmt.Errorf("hub-id flag not set"), "unable to start manager")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "mcoa-cmo-config.open-cluster-management.io",
		Cache:                  mcoa.GetCacheOptions(),
		WebhookServer:          ctrlwebhook.NewServer(ctrlwebhook.Options{Port: 9443}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	hubInfo := &operatorconfig.HubInfo{
		AlertmanagerEndpoint: hubAmURL,
		HubClusterID:         hubID,
	}

	if err = mcoa.NewMCOAAgentReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName("MCOA-Agent"),
		mgr.GetScheme(),
		mgr.GetEventRecorderFor("mcoa-agent-controller"),
		namespace,
		clusterID,
		hubInfo,
		hubAmCASecret,
		hubAmCertSecret,
		hubAmAccessorSecret,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MCOA-Agent")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	setupLog.Info("starting mcoa manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func runLegacy(args []string) {
	setupLog.Info("Starting legacy mode")
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8383", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	klog.InitFlags(flag.CommandLine)
	_ = flag.CommandLine.Parse(args)

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
