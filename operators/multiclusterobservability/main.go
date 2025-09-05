// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"

	imagev1 "github.com/openshift/api/image/v1"

	imagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/IBM/controller-filtered-cache/filteredcache"
	ocinfrav1 "github.com/openshift/api/config/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlruntimescheme "sigs.k8s.io/controller-runtime/pkg/scheme"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	migrationv1alpha1 "sigs.k8s.io/kube-storage-version-migrator/pkg/apis/migration/v1alpha1"

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	observatoriumAPIs "github.com/stolostron/observatorium-operator/api/v1alpha1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	workv1 "open-cluster-management.io/api/work/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	observabilityv1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	observabilityv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/rightsizing"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/webhook"
	operatorsutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	// +kubebuilder:scaffold:imports
)

var (
	metricsHost       = "0.0.0.0"
	metricsPort int32 = 8383

	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(observabilityv1beta1.AddToScheme(scheme))
	utilruntime.Must(observabilityv1beta2.AddToScheme(scheme))
	utilruntime.Must(observatoriumAPIs.AddToScheme(scheme))
	utilruntime.Must(prometheusv1.AddToScheme(scheme))
	utilruntime.Must(addonv1alpha1.AddToScheme(scheme))
	utilruntime.Must(imagev1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(monitoringv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	// var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var webhookPort int
	// flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&webhookPort, "webhook-server-port", 9443, "The listening port of the webhook server.")
	opts := zap.Options{
		// enable development mode for more human-readable output, extra stack traces and logging information, etc
		// disable this in final release
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	crdClient, err := operatorsutil.GetOrCreateCRDClient()
	if err != nil {
		setupLog.Error(err, "Failed to create the CRD client")
		os.Exit(1)
	}

	// Add route Openshift scheme
	if err := routev1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := oauthv1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := ocinfrav1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := operatorv1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := workv1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := clusterv1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := clusterv1beta1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := policyv1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	ingressCtlCrdExists, err := operatorsutil.CheckCRDExist(crdClient, config.IngressControllerCRD)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	mchCrdExists, err := operatorsutil.CheckCRDExist(crdClient, config.MCHCrdName)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}
	if mchCrdExists {
		if err := mchv1.SchemeBuilder.AddToScheme(scheme); err != nil {
			setupLog.Error(err, "")
			os.Exit(1)
		}
	}

	// add scheme of storage version migration
	if err := migrationv1alpha1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := addonv1alpha1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	mcoNamespace := config.GetMCONamespace()
	gvkLabelsMap := map[schema.GroupVersionKind][]filteredcache.Selector{
		corev1.SchemeGroupVersion.WithKind("Secret"): {
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.OpenshiftIngressOperatorNamespace)},
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.OpenshiftIngressNamespace)},
		},
		corev1.SchemeGroupVersion.WithKind("ConfigMap"): {
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
		},
		corev1.SchemeGroupVersion.WithKind("Service"): {
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
		},
		corev1.SchemeGroupVersion.WithKind("ServiceAccount"): {
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
		},
		appsv1.SchemeGroupVersion.WithKind("Deployment"): {
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
		},
		appsv1.SchemeGroupVersion.WithKind("StatefulSet"): {
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
		},
		workv1.SchemeGroupVersion.WithKind("ManifestWork"): {
			{LabelSelector: "owner==multicluster-observability-operator"},
		},
		clusterv1.SchemeGroupVersion.WithKind("ManagedCluster"): {
			{LabelSelector: "vendor!=auto-detect,observability!=disabled"},
		},
		addonv1alpha1.SchemeGroupVersion.WithKind("ClusterManagementAddOn"): {
			{FieldSelector: fmt.Sprintf("metadata.name=%s", util.ObservabilityController)},
		},
		addonv1alpha1.SchemeGroupVersion.WithKind("ManagedClusterAddOn"): {
			{FieldSelector: fmt.Sprintf("metadata.name=%s", config.ManagedClusterAddonName)},
		},
	}

	if ingressCtlCrdExists {
		gvkLabelsMap[operatorv1.SchemeGroupVersion.WithKind("IngressController")] = []filteredcache.Selector{
			{
				FieldSelector: fmt.Sprintf(
					"metadata.namespace==%s,metadata.name==%s",
					config.OpenshiftIngressOperatorNamespace,
					config.OpenshiftIngressOperatorCRName,
				),
			},
		}
	}
	if mchCrdExists {
		gvkLabelsMap[mchv1.GroupVersion.WithKind("MultiClusterHub")] = []filteredcache.Selector{
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", mcoNamespace)},
		}
	}

	// The following RBAC resources will not be watched by MCO, the selector will not impact the mco behavior, which
	// means MCO will fetch kube-apiserver for the correspoding resource if the resource can't be found in the cache.
	// Adding selector will reduce the cache size when the managedcluster scale.
	gvkLabelsMap[rbacv1.SchemeGroupVersion.WithKind("ClusterRole")] = []filteredcache.Selector{
		{LabelSelector: "owner==multicluster-observability-operator"},
	}
	gvkLabelsMap[rbacv1.SchemeGroupVersion.WithKind("ClusterRoleBinding")] = []filteredcache.Selector{
		{LabelSelector: "owner==multicluster-observability-operator"},
	}
	gvkLabelsMap[rbacv1.SchemeGroupVersion.WithKind("Role")] = []filteredcache.Selector{
		{LabelSelector: "owner==multicluster-observability-operator"},
	}
	gvkLabelsMap[rbacv1.SchemeGroupVersion.WithKind("RoleBinding")] = []filteredcache.Selector{
		{LabelSelector: "owner==multicluster-observability-operator"},
	}

	// Add filter for ManagedClusterAddOn to reduce the cache size when the managedclusters scale.
	gvkLabelsMap[addonv1alpha1.SchemeGroupVersion.WithKind("ManagedClusterAddOn")] = []filteredcache.Selector{
		{LabelSelector: "owner==multicluster-observability-operator"},
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort)},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "b9d51391.open-cluster-management.io",
		NewCache:               filteredcache.NewEnhancedFilteredCacheBuilder(gvkLabelsMap),
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port: webhookPort,
			TLSOpts: []func(*tls.Config){
				func(t *tls.Config) {
					t.MinVersion = tls.VersionTLS12
				},
			},
		}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = operatorsutil.UpdateCRDWebhookNS(crdClient, mcoNamespace, config.MCOCrdName); err != nil {
		setupLog.Error(
			err,
			"unable to update webhook service namespace in MCO CRD",
			"controller",
			"MultiClusterObservability",
		)
	}

	svmCrdExists, err := operatorsutil.CheckCRDExist(crdClient, config.StorageVersionMigrationCrdName)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	mcghCrdExists, err := operatorsutil.CheckCRDExist(crdClient, config.MCGHCrdName)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	scrapeConfigCrdExists, err := operatorsutil.CheckCRDExist(crdClient, config.PrometheusScrapeConfigsCrdName)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	crdMaps := map[string]bool{
		config.MCHCrdName:                     mchCrdExists,
		config.StorageVersionMigrationCrdName: svmCrdExists,
		config.IngressControllerCRD:           ingressCtlCrdExists,
		config.MCGHCrdName:                    mcghCrdExists,
		config.PrometheusScrapeConfigsCrdName: scrapeConfigCrdExists,
	}

	imageClient, err := imagev1client.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		setupLog.Error(err, "failed to create openshift image client")
		os.Exit(1)
	}

	if err = (&mcoctrl.MultiClusterObservabilityReconciler{
		Manager:     mgr,
		Client:      mgr.GetClient(),
		Log:         ctrl.Log.WithName("controllers").WithName("MultiClusterObservability"),
		Scheme:      mgr.GetScheme(),
		CRDMap:      crdMaps,
		APIReader:   mgr.GetAPIReader(),
		RESTMapper:  mgr.GetRESTMapper(),
		ImageClient: imageClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MultiClusterObservability")
		os.Exit(1)
	}

	if err = (&rightsizing.RightSizingReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("RightSizing"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RightSizing")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}
	if err := operatorsutil.RegisterDebugEndpoint(mgr.AddMetricsServerExtraHandler); err != nil {
		setupLog.Error(err, "unable to set up debug handler")
		os.Exit(1)
	}

	// Setup Scheme for observatorium resources
	schemeBuilder := &ctrlruntimescheme.Builder{
		GroupVersion: schema.GroupVersion{
			Group:   "core.observatorium.io",
			Version: "v1alpha1",
		},
	}
	schemeBuilder.Register(&observatoriumAPIs.Observatorium{}, &observatoriumAPIs.ObservatoriumList{})
	if err := schemeBuilder.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err = (&observabilityv1beta2.MultiClusterObservability{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "MultiClusterObservability")
		os.Exit(1)
	}

	setupLog.Info("add webhook controller to manager")
	if err := mgr.Add(webhook.NewWebhookController(mgr.GetClient(), nil, config.GetValidatingWebhookConfigurationForMCO())); err != nil {
		setupLog.Error(err, "unable to add webhook controller to manager")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
