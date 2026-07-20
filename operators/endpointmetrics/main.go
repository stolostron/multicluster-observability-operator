// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"github.com/IBM/controller-filtered-cache/filteredcache"
	ocinfrav1 "github.com/openshift/api/config/v1"
	tlsutil "github.com/openshift/controller-runtime-common/pkg/tls"
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
	mcoaRunner     = runMCOA
	standardRunner = runStandard
	cleanupRunner  = runCleanup
)

func main() {
	printVersion()
	execute(os.Args)
}

func execute(args []string) {
	cmd := "standard"
	if len(args) > 1 {
		cmd = args[1]
	}

	switch cmd {
	case "mcoa":
		// len(args) >= 2 is guaranteed since args[1] matches "mcoa"
		mcoaRunner(args[2:])
	case "standard", "legacy":
		// Guard against len(args) == 1 (no arguments) defaulting to standard mode
		subArgs := []string{}
		if len(args) > 2 {
			subArgs = args[2:]
		}
		standardRunner(subArgs)
	case "cleanup":
		// len(args) >= 2 is guaranteed since args[1] matches "cleanup"
		cleanupRunner(args[2:])
	default:
		// default to standard for backward compatibility if argument is just a flag
		// len(args) >= 2 is guaranteed since len(args) > 1 and it did not match subcommands above
		standardRunner(args[1:])
	}
}

func runCleanup(args []string) {
	ctrl.SetLogger(klog.NewKlogr())
	setupLog.Info("Starting MCOA cleanup")
	if err := doCleanup(args); err != nil {
		setupLog.Error(err, "best-effort cleanup incomplete")
		os.Exit(1)
	}
	setupLog.Info("Cleanup completed successfully")
}

func doCleanup(args []string) error {
	fs := flag.NewFlagSet("cleanup", flag.ExitOnError)
	var clusterName string
	var hubAmCASecret string

	fs.StringVar(&clusterName, "cluster-name", "", "The name of the managed cluster to clean up (optional).")
	fs.StringVar(&hubAmCASecret, "hub-alertmanager-ca-secret", "", "The name of the CA secret for the Hub's Alertmanager to clean up.")
	klog.InitFlags(fs)
	// Parse will exit on error due to flag.ExitOnError, so we can ignore the error return value.
	_ = fs.Parse(args)

	if hubAmCASecret == "" {
		return fmt.Errorf("hub-alertmanager-ca-secret flag not set")
	}

	cfg := ctrl.GetConfigOrDie()
	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("unable to create client for cleanup: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var errs []error

	setupLog.Info("Reverting Platform monitoring configuration", "caSecret", hubAmCASecret)
	if err := obsepctl.RevertClusterMonitoringConfig(ctx, cl, hubAmCASecret, clusterName); err != nil {
		setupLog.Error(err, "failed to revert platform monitoring config")
		errs = append(errs, err)
	}

	if ctx.Err() != nil {
		setupLog.Error(ctx.Err(), "cleanup context canceled or timed out, aborting remaining steps")
		errs = append(errs, ctx.Err())
		return fmt.Errorf("cleanup failed: %w", errors.Join(errs...))
	}

	setupLog.Info("Reverting User Workload monitoring configuration", "caSecret", hubAmCASecret)
	if err := obsepctl.RevertUserWorkloadMonitoringConfig(ctx, cl, hubAmCASecret); err != nil {
		setupLog.Error(err, "failed to revert user workload monitoring config")
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup failed: %w", errors.Join(errs...))
	}

	return nil
}

func runMCOA(args []string) {
	setupLog.Info("Starting MCOA mode")
	fs := flag.NewFlagSet("mcoa", flag.ExitOnError)
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var hubAmURL string
	var clusterID string
	var clusterName string
	var namespace string
	var hubAmCASecret string
	var hubAmCertSecret string
	var hubAmAccessorSecret string
	var enableUWLAlertForwarding bool

	fs.StringVar(&metricsAddr, "metrics-bind-address", ":8383", "The address the metric endpoint binds to.")
	fs.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&hubAmURL, "hub-alertmanager-url", "", "The URL of the Hub's Alertmanager.")
	fs.StringVar(&clusterID, "cluster-id", "", "The ID of the managed cluster.")
	fs.StringVar(&clusterName, "cluster-name", "", "The name of the managed cluster.")
	fs.StringVar(&namespace, "namespace", "", "The namespace the operator is running in.")
	fs.StringVar(
		&hubAmCASecret,
		"hub-alertmanager-ca-secret",
		"",
		"The name of the CA secret for the Hub's Alertmanager. This flag is required even when alert forwarding is disabled with MCOA to correctly revert the cluster-monitoring-config ConfigMap.",
	)
	fs.StringVar(&hubAmCertSecret, "hub-alertmanager-cert-secret", "", "The name of the TLS cert/key secret for the Hub's Alertmanager.")
	fs.StringVar(&hubAmAccessorSecret, "hub-alertmanager-accessor-secret", "", "The name of the accessor token secret for the Hub's Alertmanager.")
	fs.BoolVar(&enableUWLAlertForwarding, "enable-uwl-alert-forwarding", true, "Enable or disable forwarding of user workload monitoring alerts.")

	klog.InitFlags(fs)
	// Parse will exit on error due to flag.ExitOnError, so we can ignore the error return value.
	_ = fs.Parse(args)

	ctrl.SetLogger(klog.NewKlogr())

	crdClient, err := operatorsutil.GetOrCreateCRDClient()
	if err != nil {
		setupLog.Error(err, "Failed to create the CRD client")
		os.Exit(1)
	}

	if namespace == "" {
		setupLog.Error(fmt.Errorf("namespace flag not set"), "unable to start manager")
		os.Exit(1)
	}

	if hubAmURL != "" {
		parsedURL, err := url.Parse(hubAmURL)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			setupLog.Error(fmt.Errorf("invalid hub-alertmanager-url %q: must be a valid absolute URL", hubAmURL), "unable to start manager")
			os.Exit(1)
		}

		if clusterID == "" {
			setupLog.Error(fmt.Errorf("cluster-id flag not set"), "unable to start manager")
			os.Exit(1)
		}

		if clusterName == "" {
			setupLog.Error(fmt.Errorf("cluster-name flag not set"), "unable to start manager")
			os.Exit(1)
		}

		if hubAmCASecret == "" {
			setupLog.Error(fmt.Errorf("hub-alertmanager-ca-secret flag not set"), "unable to start manager")
			os.Exit(1)
		}

		if hubAmCertSecret == "" {
			setupLog.Error(fmt.Errorf("hub-alertmanager-cert-secret flag not set"), "unable to start manager")
			os.Exit(1)
		}

		if hubAmAccessorSecret == "" {
			setupLog.Error(fmt.Errorf("hub-alertmanager-accessor-secret flag not set"), "unable to start manager")
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithCancel(ctrl.SetupSignalHandler())

	tlsConfig, err := operatorsutil.GetOrCreateTLSConfig(ctx)
	if err != nil {
		setupLog.Error(err, "unable to get OCP TLS config")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress:   metricsAddr,
			TLSOpts:       []func(*tls.Config){tlsConfig},
			SecureServing: true,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "mcoa-cmo-config.open-cluster-management.io",
		Cache:                  mcoa.GetCacheOptions(),
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port:    9443,
			TLSOpts: []func(*tls.Config){tlsConfig},
		}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = mcoa.NewMCOAAgentReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName("mcoa-endpoint-controller"),
		mgr.GetScheme(),
		mgr.GetEventRecorder("mcoa-endpoint-controller"),
		namespace,
		clusterID,
		clusterName,
		hubAmURL,
		hubAmCASecret,
		hubAmCertSecret,
		hubAmAccessorSecret,
		enableUWLAlertForwarding,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "mcoa-endpoint-controller")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	apiServerCrdExists, err := operatorsutil.CheckCRDExist(crdClient, operatorconfig.OCPApiServerCrdName)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}
	if apiServerCrdExists {
		tlsProfileSpec, err := operatorsutil.GetOrCreateTLSProfileSpec(ctx)
		if err != nil {
			setupLog.Info("unable to get TLS profile spec, skipping SecurityProfileWatcher", "error", err)
		} else {
			if err = (&tlsutil.SecurityProfileWatcher{
				Client:                mgr.GetClient(),
				InitialTLSProfileSpec: *tlsProfileSpec,
				OnProfileChange: func(ctx context.Context, oldTLSProfileSpec, newTLSProfileSpec ocinfrav1.TLSProfileSpec) {
					setupLog.Info("TLS profile changed, shutting the manager down to reload",
						"oldProfile", oldTLSProfileSpec,
						"newProfile", newTLSProfileSpec,
					)
					cancel()
				},
			}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create TLS security profile watcher")
				os.Exit(1)
			}
		}
	}

	setupLog.Info("starting mcoa manager")
	if err := mgr.Start(ctx); err != nil {
		cancel()
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
	cancel()
}

func runStandard(args []string) {
	ctrl.SetLogger(klog.NewKlogr())
	setupLog.Info("Starting standard mode")
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8383", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	klog.InitFlags(flag.CommandLine)
	// flag.CommandLine is configured to exit on error (flag.ExitOnError), so we can ignore the error return value.
	_ = flag.CommandLine.Parse(args)

	crdClient, err := operatorsutil.GetOrCreateCRDClient()
	if err != nil {
		setupLog.Error(err, "Failed to create the CRD client")
		os.Exit(1)
	}

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

	ctx, cancel := context.WithCancel(ctrl.SetupSignalHandler())

	tlsConfig, err := operatorsutil.GetOrCreateTLSConfig(ctx)
	if err != nil {
		setupLog.Error(err, "unable to get OCP TLS config or default")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress:   metricsAddr,
			TLSOpts:       []func(*tls.Config){tlsConfig},
			SecureServing: true,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "7c30ca38.open-cluster-management.io",
		NewCache:               filteredcache.NewEnhancedFilteredCacheBuilder(gvkLabelMap),
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port:    9443,
			TLSOpts: []func(*tls.Config){tlsConfig},
		}),
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

	apiServerCrdExists, err := operatorsutil.CheckCRDExist(crdClient, operatorconfig.OCPApiServerCrdName)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}
	if apiServerCrdExists {
		tlsProfileSpec, err := operatorsutil.GetOrCreateTLSProfileSpec(ctx)
		if err != nil {
			setupLog.Info("unable get TLS profile spec, skipping SecurityProfileWatcher", "error", err)
		} else {
			if err = (&tlsutil.SecurityProfileWatcher{
				Client:                mgr.GetClient(),
				InitialTLSProfileSpec: *tlsProfileSpec,
				OnProfileChange: func(ctx context.Context, oldTLSProfileSpec, newTLSProfileSpec ocinfrav1.TLSProfileSpec) {
					setupLog.Info("TLS profile changed, shutting the manager down to reload",
						"oldProfile", oldTLSProfileSpec,
						"newProfile", newTLSProfileSpec,
					)
					cancel()
				},
			}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create TLS security profile watcher")
				os.Exit(1)
			}
		}
	}

	// start lease
	util.StartLease()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		cancel()
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
	cancel()
}
