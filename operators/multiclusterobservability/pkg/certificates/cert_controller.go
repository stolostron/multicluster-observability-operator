// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"reflect"
	"slices"
	"time"

	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	restartLabel = "cert/time-restarted"
)

var (
	caSecretNames            = []string{serverCACerts, clientCACerts}
	isCertControllerRunnning = false
)

func Start(c client.Client, ingressCtlCrdExists bool) {
	if isCertControllerRunnning {
		return
	}
	isCertControllerRunnning = true

	// setup ocm addon manager
	addonMgr, err := addonmanager.New(ctrl.GetConfigOrDie())
	if err != nil {
		log.Error(err, "Failed to init addon manager")
		os.Exit(1)
	}
	agent := &ObservabilityAgent{client: c}
	err = addonMgr.AddAgent(agent)
	if err != nil {
		log.Error(err, "Failed to add agent for addon manager")
		os.Exit(1)
	}

	err = addonMgr.Start(context.TODO())
	if err != nil {
		log.Error(err, "Failed to start addon manager")
		os.Exit(1)
	}

	kubeClient, err := kubernetes.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		log.Error(err, "Failed to create kube client")
		os.Exit(1)
	}
	watchlist := cache.NewListWatchFromClient(
		kubeClient.CoreV1().RESTClient(),
		"secrets",
		config.GetDefaultNamespace(),
		fields.OneTermEqualSelector("metadata.namespace", config.GetDefaultNamespace()),
	)
	options := cache.InformerOptions{
		ListerWatcher: watchlist,
		ObjectType:    &v1.Secret{},
		ResyncPeriod:  time.Minute * 60,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    onAdd(c),
			DeleteFunc: onDelete(c),
			UpdateFunc: onUpdate(c, ingressCtlCrdExists),
		},
	}
	_, controller := cache.NewInformerWithOptions(options)

	stop := make(chan struct{})
	go controller.Run(stop)
}

func restartPods(c client.Client, s v1.Secret, isUpdate bool) {
	if config.GetMonitoringCRName() == "" {
		return
	}
	dName := ""
	// No need to restart the rbac-query-proxy, it auto reloads the mTLS config when changed.
	if s.Name == config.ClientCACerts || s.Name == config.ServerCerts {
		dName = config.GetOperandName(config.ObservatoriumAPI)
	}
	if s.Name == hubMetricsCollectorMtlsCert {
		dName = config.HubMetricsCollectorName
	}
	if dName != "" {
		updateDeployLabel(c, dName, isUpdate)
	}
}

func updateDeployLabel(c client.Client, dName string, isUpdate bool) {
	dep := &appv1.Deployment{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      dName,
		Namespace: config.GetDefaultNamespace(),
	}, dep)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to check the deployment", "name", dName)
		}
		return
	}
	if isUpdate || dep.Status.ReadyReplicas != 0 {
		newDep := dep.DeepCopy()
		newDep.Spec.Template.ObjectMeta.Labels[restartLabel] = time.Now().Format("2006-1-2.150405")
		err := c.Patch(context.TODO(), newDep, client.StrategicMergeFrom(dep))
		if err != nil {
			log.Error(err, "Failed to update the deployment", "name", dName)
		} else {
			log.Info("Update deployment cert/restart label", "name", dName)
		}
	}
}

func needsRenew(s v1.Secret) bool {
	certSecretNames := []string{serverCACerts, clientCACerts, serverCerts, grafanaCerts, hubMetricsCollectorMtlsCert}
	if !slices.Contains(certSecretNames, s.Name) {
		return false
	}
	data := s.Data["tls.crt"]
	if len(data) == 0 {
		log.Info("miss cert, need to recreate", "name", s.Name)
		return true
	}
	block, _ := pem.Decode(data)
	certs, err := x509.ParseCertificates(block.Bytes)
	if err != nil {
		log.Error(err, "wrong certificate found, need to recreate", "name", s.Name)
		return true
	}
	cert := certs[0]
	maxWait := cert.NotAfter.Sub(cert.NotBefore) / 5
	latestTime := cert.NotAfter.Add(-maxWait)
	if time.Now().After(latestTime) {
		log.Info(fmt.Sprintf("certificate expired in %6.3f hours, need to renew",
			time.Until(cert.NotAfter).Hours()), "secret", s.Name)
		return true
	}

	return false
}

func onAdd(c client.Client) func(obj any) {
	return func(obj any) {
		restartPods(c, *obj.(*v1.Secret), false)
	}
}

func onDelete(c client.Client) func(obj any) {
	return func(obj any) {
		s := *obj.(*v1.Secret)
		if slices.Contains(caSecretNames, s.Name) {
			mco := &mcov1beta2.MultiClusterObservability{}
			err := c.Get(context.TODO(), types.NamespacedName{
				Name: config.GetMonitoringCRName(),
			}, mco)
			if err == nil {
				log.Info(
					"secret for ca certificate deleted by mistake, add the cert back to the new created one",
					"name",
					s.Name,
				)
				i := 0
				for {
					caSecret := &v1.Secret{}
					err = c.Get(context.TODO(), types.NamespacedName{
						Name:      s.Name,
						Namespace: config.GetDefaultNamespace(),
					}, caSecret)
					if err == nil {
						caSecret.Data["tls.crt"] = append(caSecret.Data["tls.crt"], s.Data["tls.crt"]...)
						err = c.Update(context.TODO(), caSecret)
						if err != nil {
							log.Error(err, "Failed to update secret for ca certificate", "name", s.Name)
							i++
						} else {
							break
						}
					} else {
						// wait mco operator recreate the ca certificate at most 30 seconds
						if i < 6 {
							time.Sleep(5 * time.Second)
							i++
						} else {
							log.Info("new secret for ca certificate not created")
							break
						}
					}
				}
			}
		}
	}
}

func onUpdate(c client.Client, ingressCtlCrdExists bool) func(oldObj, newObj any) {
	return func(oldObj, newObj any) {
		oldS := *oldObj.(*v1.Secret)
		newS := *newObj.(*v1.Secret)
		if !reflect.DeepEqual(oldS.Data, newS.Data) {
			restartPods(c, newS, true)
		} else {
			if slices.Contains(caSecretNames, newS.Name) {
				removeExpiredCA(c, newS.Name)
			}
			if needsRenew(newS) {
				var err error
				var hosts []string
				switch name := newS.Name; {
				case name == serverCACerts:
					err, _ = createCASecret(c, nil, nil, true, serverCACerts, serverCACertifcateCN)
				case name == clientCACerts:
					err, _ = createCASecret(c, nil, nil, true, clientCACerts, clientCACertificateCN)
				case name == grafanaCerts:
					err = createCertSecret(c, nil, nil, true, grafanaCerts, false, grafanaCertificateCN, nil, nil, nil)
				case name == serverCerts:
					hosts, err = getHosts(c, ingressCtlCrdExists)
					if err == nil {
						err = createCertSecret(c, nil, nil, true, serverCerts, true, serverCertificateCN, nil, hosts, nil)
					}
				case name == hubMetricsCollectorMtlsCert:
					// ACM 8509: Special case for hub metrics collector
					// Delete the MTLS secret and the placement controller will reconcile to create a new one
					HubMtlsSecret := &v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      operatorconfig.HubMetricsCollectorMtlsCert,
							Namespace: config.GetDefaultNamespace(),
						},
					}
					err = c.Delete(context.Background(), HubMtlsSecret)
				default:
					return
				}
				if err != nil {
					log.Error(err, "Failed to renew the certificate", "name", newS.Name)
				}
			}
		}
	}
}
