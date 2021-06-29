// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/IBM/controller-filtered-cache/filteredcache"
	ocinfrav1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlruntimescheme "sigs.k8s.io/controller-runtime/pkg/scheme"
	migrationv1alpha1 "sigs.k8s.io/kube-storage-version-migrator/pkg/apis/migration/v1alpha1"

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	placementv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	observabilityv1beta1 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta1"
	observabilityv1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	mcoctrl "github.com/open-cluster-management/multicluster-observability-operator/controllers/multiclusterobservability"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
	mchv1 "github.com/open-cluster-management/multiclusterhub-operator/pkg/apis/operator/v1"
	observatoriumAPIs "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

var (
	metricsHost               = "0.0.0.0"
	metricsPort         int32 = 8383
	operatorMetricsPort int32 = 8686

	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(observabilityv1beta1.AddToScheme(scheme))
	utilruntime.Must(observabilityv1beta2.AddToScheme(scheme))
	utilruntime.Must(observatoriumAPIs.AddToScheme(scheme))
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
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	crdClient, err := util.GetOrCreateCRDClient()
	if err != nil {
		setupLog.Error(err, "Failed to create the CRD client")
		os.Exit(1)
	}

	// Add route Openshift scheme
	if err := routev1.AddToScheme(scheme); err != nil {
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

	placementRuleCrdExists, err := util.CheckCRDExist(crdClient, config.PlacementRuleCrdName)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}
	if placementRuleCrdExists {
		if err := placementv1.AddToScheme(scheme); err != nil {
			setupLog.Error(err, "")
			os.Exit(1)
		}
	}

	mchCrdExists, err := util.CheckCRDExist(crdClient, config.MCHCrdName)
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
		v1.SchemeGroupVersion.WithKind("Secret"): []filteredcache.Selector{
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.OpenshiftIngressOperatorNamespace)},
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.OpenshiftIngressNamespace)},
		},
		v1.SchemeGroupVersion.WithKind("ConfigMap"): []filteredcache.Selector{
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
		},
		v1.SchemeGroupVersion.WithKind("Service"): []filteredcache.Selector{
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
		},
		v1.SchemeGroupVersion.WithKind("ServiceAccount"): []filteredcache.Selector{
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
		},
		appsv1.SchemeGroupVersion.WithKind("Deployment"): []filteredcache.Selector{
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
		},
		appsv1.SchemeGroupVersion.WithKind("StatefulSet"): []filteredcache.Selector{
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
		},
		workv1.SchemeGroupVersion.WithKind("ManifestWork"): []filteredcache.Selector{
			{LabelSelector: "owner==multicluster-observability-operator"},
		},
		operatorv1.SchemeGroupVersion.WithKind("IngressController"): []filteredcache.Selector{
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s,metadata.name==%s", config.OpenshiftIngressOperatorNamespace, config.OpenshiftIngressOperatorCRName)},
		},
	}

	if placementRuleCrdExists {
		gvkLabelsMap[placementv1.SchemeGroupVersion.WithKind("PlacementRule")] = []filteredcache.Selector{
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", config.GetDefaultNamespace())},
		}
	}

	if mchCrdExists {
		gvkLabelsMap[mchv1.SchemeGroupVersion.WithKind("MultiClusterHub")] = []filteredcache.Selector{
			{FieldSelector: fmt.Sprintf("metadata.namespace==%s", mcoNamespace)},
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Port:                   webhookPort,
		Scheme:                 scheme,
		MetricsBindAddress:     fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "b9d51391.open-cluster-management.io",
		NewCache:               filteredcache.NewEnhancedFilteredCacheBuilder(gvkLabelsMap),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = util.UpdateCRDWebhookNS(crdClient, mcoNamespace, config.MCOCrdName); err != nil {
		setupLog.Error(err, "unable to update webhook service namespace in MCO CRD", "controller", "MultiClusterObservability")
	}

	svmCrdExists, err := util.CheckCRDExist(crdClient, config.StorageVersionMigrationCrdName)
	if err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	crdMaps := map[string]bool{
		config.MCHCrdName:                     mchCrdExists,
		config.PlacementRuleCrdName:           placementRuleCrdExists,
		config.StorageVersionMigrationCrdName: svmCrdExists,
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
		setupLog.Error(err, "unable to create webhook", "webhook", "Captain")
		os.Exit(1)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	// wait the manifestwork CRD for no more than 10 minutes
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*600)
	defer cancel()

	go func(ctx context.Context) {
		setupLog.Info("checking the manifestworks.work.open-cluster-management.io CRD...")
		for {
			select {
			case <-ctx.Done():
				setupLog.Error(fmt.Errorf("timeout waiting for the manifestworks.work.open-cluster-management.io CRD"), "CRD manifestworks.work.open-cluster-management.io not found after 10 minutes.")
				os.Exit(1)
			case <-time.After(time.Second * 5):
				if mchCrdExists, _ := util.CheckCRDExist(crdClient, config.ManifestWorkCrdName); mchCrdExists {
					wg.Done()
					return
				}
			}
		}
	}(ctx)

	// wait for the ManifestWork CRD is created before starting the manager
	wg.Wait()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
