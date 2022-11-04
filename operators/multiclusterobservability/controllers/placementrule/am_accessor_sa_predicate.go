// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package placementrule

import (
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	config "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
)

func getAlertManagerAccessorSAPred(c client.Client, ingressCtlCrdExists bool) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetName() == config.AlertmanagerAccessorSAName &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				// wait 10s for access_token of alertmanager and generate the secret that contains the access_token
				/* #nosec */
				wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
					var err error
					log.Info("generate amAccessorTokenSecret for alertmanager access serviceaccount CREATE")
					if amAccessorTokenSecret, err = generateAmAccessorTokenSecret(c); err == nil {
						return true, nil
					}
					return false, err
				})
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if (e.ObjectNew.GetName() == config.AlertmanagerAccessorSAName &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace()) &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// regenerate the secret that contains the access_token for the Alertmanager in the Hub cluster
				amAccessorTokenSecret, _ = generateAmAccessorTokenSecret(c)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}
