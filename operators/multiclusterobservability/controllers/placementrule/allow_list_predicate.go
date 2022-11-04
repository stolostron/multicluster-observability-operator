// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project.
package placementrule

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	config "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
)

func getAllowlistPred(c client.Client, ingressCtlCrdExists bool) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if (e.Object.GetName() == config.AllowlistCustomConfigMapName ||
				e.Object.GetName() == operatorconfig.AllowlistConfigMapName) &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				// generate the metrics allowlist configmap
				log.Info("generate metric allow list configmap for allowlist configmap CREATE")
				metricsAllowlistConfigMap, ocp311metricsAllowlistConfigMap, _ = generateMetricsListCM(c)
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if (e.ObjectNew.GetName() == config.AllowlistCustomConfigMapName ||
				e.ObjectNew.GetName() == operatorconfig.AllowlistConfigMapName) &&
				e.ObjectNew.GetNamespace() == config.GetDefaultNamespace() &&
				e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion() {
				// regenerate the metrics allowlist configmap
				log.Info("generate metric allow list configmap for allowlist configmap UPDATE")
				metricsAllowlistConfigMap, ocp311metricsAllowlistConfigMap, _ = generateMetricsListCM(c)
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if (e.Object.GetName() == config.AllowlistCustomConfigMapName ||
				e.Object.GetName() == operatorconfig.AllowlistConfigMapName) &&
				e.Object.GetNamespace() == config.GetDefaultNamespace() {
				// regenerate the metrics allowlist configmap
				log.Info("generate metric allow list configmap for allowlist configmap UPDATE")
				metricsAllowlistConfigMap, ocp311metricsAllowlistConfigMap, _ = generateMetricsListCM(c)
				return true
			}
			return false
		},
	}
}
