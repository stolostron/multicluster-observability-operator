// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"github.com/IBM/controller-filtered-cache/filteredcache"
	ocinfrav1 "github.com/openshift/api/config/v1"
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"go.uber.org/zap/zapcore"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	obsepctl "github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	statusctl "github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/status"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/version"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	operatorsutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = k8sruntime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(oav1beta1.AddToScheme(scheme))
	utilruntime.Must(ocinfrav1.AddToScheme(scheme))
	utilruntime.Must(prometheusv1.AddToScheme(scheme))
	utilruntime.Must(hyperv1.AddToScheme(scheme))
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
	opts := zap.Options{
		// enable development mode for more human-readable output, extra stack traces and logging information, etc
		// disable this in final release
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	namespaceSelector := fmt.Sprintf("metadata.namespace==%s", os.Getenv("WATCH_NAMESPACE"))
	gvkLabelMap := map[schema.GroupVersionKind][]filteredcache.Selector{
		v1.SchemeGroupVersion.WithKind("Secret"): []filteredcache.Selector{
			{FieldSelector: namespaceSelector},
		},
		v1.SchemeGroupVersion.WithKind("ConfigMap"): []filteredcache.Selector{
			{FieldSelector: namespaceSelector},
			{FieldSelector: fmt.Sprintf("metadata.name==%s,metadata.namespace!=%s",
				operatorconfig.AllowlistCustomConfigMapName, "open-cluster-management-observability")},
		},
		appsv1.SchemeGroupVersion.WithKind("Deployment"): []filteredcache.Selector{
			{FieldSelector: namespaceSelector},
		},
		oav1beta1.GroupVersion.WithKind("ObservabilityAddon"): []filteredcache.Selector{
			{FieldSelector: namespaceSelector},
		},
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "7c30ca38.open-cluster-management.io",
		NewCache:               filteredcache.NewEnhancedFilteredCacheBuilder(gvkLabelMap),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	hubClient, err := util.GetOrCreateHubClient(false)
	if err != nil {
		setupLog.Error(err, "Failed to create the hub client")
		os.Exit(1)
	}

	if err = (&obsepctl.ObservabilityAddonReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		HubClient: hubClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ObservabilityAddon")
		os.Exit(1)
	}
	if err = (&statusctl.StatusReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		HubClient: hubClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Status")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := operatorsutil.RegisterDebugEndpoint(mgr.AddMetricsExtraHandler); err != nil {
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
