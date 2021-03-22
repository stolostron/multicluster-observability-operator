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
	"flag"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	certv1alpha1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	observatoriumAPIs "github.com/open-cluster-management/observatorium-operator/api/v1alpha1"
	ocinfrav1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlruntimescheme "sigs.k8s.io/controller-runtime/pkg/scheme"

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	placementv1 "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/apis/apps/v1"
	observabilityv1beta1 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta1"
	observabilityv1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
	mcoctrl "github.com/open-cluster-management/multicluster-observability-operator/controllers/multiclusterobservability"
	prctrl "github.com/open-cluster-management/multicluster-observability-operator/controllers/placementrule"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
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
	utilruntime.Must(placementv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	// var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	// flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "b9d51391.open-cluster-management.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ocpClient, err := util.CreateOCPClient()
	if err != nil {
		setupLog.Error(err, "Failed to create the OpenShift client")
		os.Exit(1)
	}
	if err = (&mcoctrl.MultiClusterObservabilityReconciler{
		Client:    mgr.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("MultiClusterObservability"),
		Scheme:    mgr.GetScheme(),
		OcpClient: ocpClient,
		APIReader: mgr.GetAPIReader(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MultiClusterObservability")
		os.Exit(1)
	}

	crdExists, err := util.CheckCRDExist("placementrules.apps.open-cluster-management.io")
	if err != nil {
		setupLog.Error(err, "Failed to check if the CRD exists")
		os.Exit(1)
	}

	if crdExists {
		if err = (&prctrl.PlacementRuleReconciler{
			Client:     mgr.GetClient(),
			Log:        ctrl.Log.WithName("controllers").WithName("PlacementRule"),
			Scheme:     mgr.GetScheme(),
			APIReader:  mgr.GetAPIReader(),
			RESTMapper: mgr.GetRESTMapper(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "PlacementRule")
			os.Exit(1)
		}
	}

	if err = (&prctrl.PlacementRuleReconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("PlacementRule"),
		Scheme:     mgr.GetScheme(),
		APIReader:  mgr.GetAPIReader(),
		RESTMapper: mgr.GetRESTMapper(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PlacementRule")
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

	// Add route Openshift scheme
	if err := routev1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := ocinfrav1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := workv1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := placementv1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := certv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	if err := addonv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "")
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

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
