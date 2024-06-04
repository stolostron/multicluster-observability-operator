// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	ocinfrav1 "github.com/openshift/api/config/v1"
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"go.uber.org/zap/zapcore"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	obsepctl "github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/observabilityendpoint"
	statusctl "github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/controllers/status"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/util"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/version"
	oav1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	oav1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
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
	opts := zap.Options{
		// enable development mode for more human-readable output, extra stack traces and logging information, etc
		// disable this in final release
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	var cacheOptions cache.Options
	cacheOptions.DefaultNamespaces = map[string]cache.Config{
		cache.AllNamespaces: {},
	}
	cacheOptions.DefaultLabelSelector = labels.Everything()
	cacheOptions.DefaultFieldSelector = fields.Everything()
	watchNS := os.Getenv("WATCH_NAMESPACE")

	cacheOptions.ByObject = map[client.Object]cache.ByObject{
		&v1.Secret{}: {
			Namespaces: map[string]cache.Config{
				watchNS: {},
			},
		},
		&v1.ConfigMap{}: {
			Namespaces: map[string]cache.Config{
				cache.AllNamespaces: {FieldSelector: fields.ParseSelectorOrDie("metadata.namespace=" + operatorconfig.AllowlistConfigMapName)},
				watchNS:             {},
			},
		},
		&appsv1.Deployment{}: {
			Namespaces: map[string]cache.Config{
				watchNS: {},
			},
		},
		&oav1beta1.ObservabilityAddon{}: {
			Namespaces: map[string]cache.Config{
				watchNS: {},
			},
		},
	}
	// Only watch MCO CRs in the hub cluster to avoid noisy log messages
	if os.Getenv("HUB_ENDPOINT_OPERATOR") == "true" {
		cacheOptions.ByObject[&oav1beta2.MultiClusterObservability{}] = cache.ByObject{
			Field: fields.ParseSelectorOrDie("metadata.name!=null"),
		}
	}

	clientConfig := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(clientConfig, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "7c30ca38.open-cluster-management.io",
		Cache:                  cacheOptions,
		Client:                 client.Options{Cache: &client.CacheOptions{Unstructured: true}},
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

	kubeClient, err := client.New(clientConfig, client.Options{Scheme: mgr.GetScheme(), Mapper: mgr.GetRESTMapper()})
	if err != nil {
		setupLog.Error(err, "Failed to create the kubernetes client")
		os.Exit(1)
	}

	if err = (&obsepctl.ObservabilityAddonReconciler{
		Client:    kubeClient,
		Scheme:    mgr.GetScheme(),
		HubClient: hubClientWithReload,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ObservabilityAddon")
		os.Exit(1)
	}

	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = os.Getenv("WATCH_NAMESPACE")
	}
	if err = (&statusctl.StatusReconciler{
		Client:       kubeClient,
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
