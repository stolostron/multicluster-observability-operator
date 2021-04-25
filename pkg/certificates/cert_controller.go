// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package certificates

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"reflect"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/open-cluster-management/addon-framework/pkg/addonmanager"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-observability-operator/pkg/util"
)

const (
	restartLabel = "cert/time-restarted"
)

func Start() {

	// setup ocm addon manager
	addonMgr, err := addonmanager.New(ctrl.GetConfigOrDie())
	if err != nil {
		log.Error(err, "Failed to init addon manager")
		os.Exit(1)
	}
	agent := &ObservabilityAgent{}
	addonMgr.AddAgent(agent)
	addonMgr.Start(context.TODO())

	kubeClient, err := kubernetes.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		log.Error(err, "Failed to create kube client")
		os.Exit(1)
	}
	watchlist := cache.NewListWatchFromClient(kubeClient.CoreV1().RESTClient(), "secrets", config.GetDefaultNamespace(),
		fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&v1.Secret{},
		time.Minute*60,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				restartPods(*kubeClient, *obj.(*v1.Secret))
			},

			DeleteFunc: func(obj interface{}) {
			},

			UpdateFunc: func(oldObj, newObj interface{}) {
				oldS := *oldObj.(*v1.Secret)
				newS := *newObj.(*v1.Secret)
				if !reflect.DeepEqual(oldS.Data, newS.Data) {
					restartPods(*kubeClient, newS)
				} else {
					if needsRenew(newS) {
						c, err := getClient()
						if err != nil {
							log.Error(err, "Failed to get client")
							return
						}
						switch name := newS.Name; {
						case name == serverCACerts:
							err = createCASecret(c, nil, nil, true, serverCACerts, serverCACertifcateCN)
						case name == clientCACerts:
							err = createCASecret(c, nil, nil, true, clientCACerts, clientCACertificateCN)
						case name == grafanaCerts:
							err = createCertSecret(c, nil, nil, true, serverCerts, true, serverCertificateCN, nil, nil, nil)
						default:
							return
						}
						if err != nil {
							log.Error(err, "Failed to renew the certificate", "name", newS.Name)
						}
					}
				}
			},
		},
	)

	stop := make(chan struct{})
	go controller.Run(stop)
}

func restartPods(c kubernetes.Clientset, s v1.Secret) {
	if config.GetMonitoringCRName() == "" {
		return
	}
	dName := ""
	if s.Name == config.ServerCACerts || s.Name == config.GrafanaCerts {
		dName = config.GetMonitoringCRName() + "-rbac-query-proxy"
	}
	if s.Name == config.ClientCACerts || s.Name == config.ServerCerts {
		dName = config.GetMonitoringCRName() + "-observatorium-api"
	}
	if dName != "" {
		updateDeployLabel(c, dName, s.ObjectMeta.CreationTimestamp.Time)
	}
}

func updateDeployLabel(c kubernetes.Clientset, dName string, sTime time.Time) {
	dep, err := c.AppsV1().Deployments(config.GetDefaultNamespace()).Get(context.TODO(), dName, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to get the deployment", "name", dName)
		return
	}
	if sTime.After(dep.ObjectMeta.CreationTimestamp.Time) {
		dep.Spec.Template.ObjectMeta.Labels[restartLabel] = time.Now().Format("2006-1-2.1504")
		_, err = c.AppsV1().Deployments(config.GetDefaultNamespace()).Update(context.TODO(), dep, metav1.UpdateOptions{})
		if err != nil {
			log.Error(err, "Failed to get the deployment", "name", dName)
		} else {
			log.Info("Update deployment cert/restart label", "name", dName)
		}
	}
}

func needsRenew(s v1.Secret) bool {
	certSecretNames := []string{serverCACerts, clientCACerts, grafanaCerts}
	if !util.Contains(certSecretNames, s.Name) {
		return false
	}
	data := s.Data["tls.crt"]
	if len(data) == 0 {
		log.Info("miss cert, need to recreate")
		return true
	}
	block, _ := pem.Decode(data)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		log.Error(err, "wrong certificate found, need to recreate")
		return true
	}
	maxWait := cert.NotAfter.Sub(cert.NotBefore) / 5
	latestTime := cert.NotAfter.Add(-maxWait)
	if time.Now().After(latestTime) {
		log.Info(fmt.Sprintf("certificate expired in %6.3f hours, need to renew",
			time.Until(cert.NotAfter).Hours()), "secret", s.Name)
		return true
	}

	return false
}
