// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	ocinfrav1 "github.com/openshift/api/config/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/util"
	"go.uber.org/zap/zapcore"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlruntimescheme "sigs.k8s.io/controller-runtime/pkg/scheme"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	migrationv1alpha1 "sigs.k8s.io/kube-storage-version-migrator/pkg/apis/migration/v1alpha1"

	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	observatoriumAPIs "github.com/stolostron/observatorium-operator/api/v1alpha1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	observabilityv1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	observabilityv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mcoctrl "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/multiclusterobservability"
	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
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

	ingressCtlCrdExists, err := operatorsutil.CheckCRDExist(crdClient, mcoconfig.IngressControllerCRD)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	mchCrdExists, err := operatorsutil.CheckCRDExist(crdClient, mcoconfig.MCHCrdName)
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

	var cacheOpts cache.Options
	cacheOpts.DefaultNamespaces = map[string]cache.Config{
		cache.AllNamespaces: {},
	}
	cacheOpts.DefaultLabelSelector = labels.Everything()
	cacheOpts.DefaultFieldSelector = fields.Everything()

	byObjectWithOwnerLabel := cache.ByObject{Label: labels.Set{"owner": "multicluster-observability-operator"}.AsSelector()}

	managedClusterLabelSelector, err := labels.Parse("vendor!=auto-detect,observability!=disabled")
	if err != nil {
		panic(err)
	}
	// The following RBAC resources will not be watched by MCO, the selector will not impact the mco behavior, which
	// means MCO will fetch kube-apiserver for the correspoding resource if the resource can't be found in the cache.
	// Adding selector will reduce the cache size when the managedcluster scale.

	mcoNamespace := mcoconfig.GetMCONamespace()
	defaultNamespace := mcoconfig.GetDefaultNamespace()

	cacheOpts.ByObject = map[client.Object]cache.ByObject{
		&corev1.Secret{}: {
			Namespaces: map[string]cache.Config{
				defaultNamespace: {},
				mcoNamespace:     {},
				mcoconfig.OpenshiftIngressOperatorNamespace: {},
				mcoconfig.OpenshiftIngressNamespace:         {},
			},
		},
		&corev1.ConfigMap{}: {
			Namespaces: map[string]cache.Config{
				defaultNamespace:       {},
				"openshift-monitoring": {},
				"kube-system":          {},
			},
		},
		&prometheusv1.ServiceMonitor{}: {
			Namespaces: map[string]cache.Config{
				"openshift-monitoring": {},
				mcoNamespace:           {},
				defaultNamespace:       {},
			},
		},
		&corev1.Service{}: {
			Namespaces: map[string]cache.Config{
				defaultNamespace: {},
			},
		},
		&corev1.Namespace{}: {},
		&corev1.ServiceAccount{}: {
			Namespaces: map[string]cache.Config{
				defaultNamespace: {},
			},
		},
		&appsv1.Deployment{}: {
			Namespaces: map[string]cache.Config{
				defaultNamespace: {},
			},
		},
		&appsv1.StatefulSet{}: {
			Namespaces: map[string]cache.Config{
				defaultNamespace: {},
			},
		},
		&workv1.ManifestWork{}: {
			Label: labels.Set{"owner": "multicluster-observability-operator"}.AsSelector(),
		},
		&clusterv1.ManagedCluster{}: {
			Label: managedClusterLabelSelector,
		},
		&addonv1alpha1.ClusterManagementAddOn{}: {
			Field: fields.Set{"metadata.name": util.ObservabilityController}.AsSelector(),
		},
		&addonv1alpha1.ManagedClusterAddOn{}: {
			Field: fields.Set{"metadata.name": util.ManagedClusterAddonName}.AsSelector(),
		},
		&rbacv1.Role{}: {
			Namespaces: map[string]cache.Config{
				defaultNamespace: {},
			},
		},
		&rbacv1.RoleBinding{}: {
			Namespaces: map[string]cache.Config{
				defaultNamespace: {},
			},
		},
		&rbacv1.ClusterRole{}:                byObjectWithOwnerLabel,
		&rbacv1.ClusterRoleBinding{}:         byObjectWithOwnerLabel,
		&addonv1alpha1.ManagedClusterAddOn{}: byObjectWithOwnerLabel,
	}

	if mchCrdExists {
		cacheOpts.ByObject[&mchv1.MultiClusterHub{}] = cache.ByObject{
			Namespaces: map[string]cache.Config{
				mcoNamespace: {},
			},
		}
	}

	if ingressCtlCrdExists {
		cacheOpts.ByObject[&operatorv1.IngressController{}] = cache.ByObject{
			Namespaces: map[string]cache.Config{
				mcoconfig.OpenshiftIngressNamespace: {
					FieldSelector: fields.ParseSelectorOrDie("metadata.name==" + mcoconfig.OpenshiftIngressOperatorCRName),
				},
			},
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort)},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "b9d51391.open-cluster-management.io",
		Cache:                  cacheOpts,
		Client:                 client.Options{Cache: &client.CacheOptions{Unstructured: true}},
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port: webhookPort,
			TLSOpts: []func(*tls.Config){
				func(t *tls.Config) {
					t.MinVersion = tls.VersionTLS12
				},
			}}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = operatorsutil.UpdateCRDWebhookNS(crdClient, mcoNamespace, mcoconfig.MCOCrdName); err != nil {
		setupLog.Error(
			err,
			"unable to update webhook service namespace in MCO CRD",
			"controller",
			"MultiClusterObservability",
		)
	}

	svmCrdExists, err := operatorsutil.CheckCRDExist(crdClient, mcoconfig.StorageVersionMigrationCrdName)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	mcghCrdExists, err := operatorsutil.CheckCRDExist(crdClient, mcoconfig.MCGHCrdName)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	crdMaps := map[string]bool{
		mcoconfig.MCHCrdName:                     mchCrdExists,
		mcoconfig.StorageVersionMigrationCrdName: svmCrdExists,
		mcoconfig.IngressControllerCRD:           ingressCtlCrdExists,
		mcoconfig.MCGHCrdName:                    mcghCrdExists,
	}
	if err = (&mcoctrl.MultiClusterObservabilityReconciler{
		Manager:    mgr,
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("MultiClusterObservability"),
		Scheme:     mgr.GetScheme(),
		CRDMap:     crdMaps,
		APIReader:  mgr.GetAPIReader(),
		RESTMapper: mgr.GetRESTMapper(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MultiClusterObservability")
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
	if err := mgr.Add(webhook.NewWebhookController(mgr.GetClient(), nil, mcoconfig.GetValidatingWebhookConfigurationForMCO())); err != nil {
		setupLog.Error(err, "unable to add webhook controller to manager")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
