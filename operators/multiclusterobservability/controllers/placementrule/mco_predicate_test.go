// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"testing"

	config "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"
)

func TestMCOPredFunc(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))
	s := scheme.Scheme
	addonv1alpha1.AddToScheme(s)
	initSchema(t)
	config.SetMonitoringCRName(mcoName)
	mco := newTestMCO()
	mcoWithAnnotation := newTestMCOWithAlertDisableAnnotation()
	pull := newTestPullSecret()

	objs := []runtime.Object{
		pull, newConsoleRoute(), newTestObsApiRoute(),
		newTestAlertmanagerRoute(), newTestIngressController(), newTestRouteCASecret(),
		newCASecret(), newCertSecret(mcoNamespace), NewMetricsAllowListCM(),
		NewAmAccessorSA(), newTestAmDefaultCA(), newManagedClusterAddon(),
	}
	cl := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()

	caseList := []struct {
		caseName       string
		disableAlerts  bool
		crdMap         map[string]bool
		expectedCreate bool
		expectedUpdate bool
		expectedDelete bool
	}{
		{
			caseName:       "no mco-disable-alerting annotation ingressCtlCrdExists=true",
			disableAlerts:  false,
			crdMap:         map[string]bool{config.IngressControllerCRD: true},
			expectedCreate: true,
			expectedUpdate: false,
			expectedDelete: true,
		},
		{
			caseName:       "no mco-disable-alerting annotation ingressCtlCrdExists=false",
			disableAlerts:  false,
			crdMap:         map[string]bool{config.IngressControllerCRD: false},
			expectedCreate: true,
			expectedUpdate: false,
			expectedDelete: true,
		},
		{
			caseName:       "mco-disable-alerting=true ingressCtlCrdExists=true",
			disableAlerts:  true,
			crdMap:         map[string]bool{config.IngressControllerCRD: true},
			expectedCreate: true,
			expectedUpdate: true,
			expectedDelete: true,
		},
		{
			caseName:       "mco-disable-alerting=true ingressCtlCrdExists=false",
			disableAlerts:  true,
			crdMap:         map[string]bool{config.IngressControllerCRD: false},
			expectedCreate: true,
			expectedUpdate: true,
			expectedDelete: true,
		},
	}

	for _, c := range caseList {
		t.Run(c.caseName, func(t *testing.T) {
			pred := getMCOPred(cl, c.crdMap)

			// create
			resetBeforeEachMCOPredTest()
			m := mco
			if c.disableAlerts {
				m = mcoWithAnnotation
			}
			ce := event.CreateEvent{
				Object: m,
			}
			if c.expectedCreate {
				if !pred.CreateFunc(ce) {
					t.Fatalf("pred func return false on applied createevent in case: (%v)", c.caseName)
				}
				// verify pull secret and hubInfoSecret are created
				if pullSecret == nil {
					t.Fatalf("pred func did not create pullSecret on applied createevent in case: (%v)", c.caseName)
				}
				if hubInfoSecret == nil {
					t.Fatalf("pred func did not create hubInfoSecret on applied createevent in case: (%v)", c.caseName)
				}
				if !c.disableAlerts && getHubInfoAlertManagerEndpoint() == "" {
					t.Fatalf("pred func alertManagerEndpoint null on applied createevent in case: (%v)", c.caseName)
				} else if c.disableAlerts && getHubInfoAlertManagerEndpoint() != "" {
					t.Fatalf("pred func alertManagerEndpoint not null on applied createevent in case: (%v)", c.caseName)
				}
			}

			// update
			resetBeforeEachMCOPredTest()
			o := mco
			n := mco
			if c.disableAlerts {
				n = mcoWithAnnotation
			}
			ue := event.UpdateEvent{
				ObjectNew: n,
				ObjectOld: o,
			}
			if c.expectedUpdate {
				if !pred.UpdateFunc(ue) {
					t.Fatalf("pred func return false on applied update in case: (%v)", c.caseName)
				}
				// verify pull secret not created and hubInfoSecret is created
				if pullSecret != nil {
					t.Fatalf("pred func created pullSecret on applied updateevent in case: (%v)", c.caseName)
				}
				if hubInfoSecret == nil {
					t.Fatalf("pred func did not create hubInfoSecret on applied updateevent in case: (%v)", c.caseName)
				}
			} else {
				if pred.UpdateFunc(ue) {
					t.Fatalf("pred func return true on applied update in case: (%v)", c.caseName)
				}
				if pullSecret != nil {
					t.Fatalf("pred func created pullSecret on applied updateevent in case: (%v)", c.caseName)
				}
				if hubInfoSecret != nil {
					t.Fatalf("pred func did not create hubInfoSecret on applied updateevent in case: (%v)", c.caseName)
				}
			}

			// delete
			resetBeforeEachMCOPredTest()
			de := event.DeleteEvent{
				Object: mco,
			}
			if c.expectedDelete {
				if !pred.DeleteFunc(de) {
					t.Fatalf("pre func return false on applied deleteevent in case: (%v)", c.caseName)
				}
			}
		})
	}
}

func resetBeforeEachMCOPredTest() {
	// reset these secrets to nil
	pullSecret, hubInfoSecret = nil, nil
	config.SetAlertingDisabled(false)
}

func getHubInfoAlertManagerEndpoint() string {
	hub := &operatorconfig.HubInfo{}
	yaml.Unmarshal(hubInfoSecret.Data[operatorconfig.HubInfoSecretKey], &hub)
	return hub.AlertmanagerEndpoint
}
