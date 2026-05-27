// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetCacheOptions returns the surgical cache options for the MCOA bridge agent.
// It whitelists only the specific ConfigMaps and Secrets required for Alertmanager configuration injection.
func GetCacheOptions() cache.Options {
	return cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&corev1.ConfigMap{}: {
				Namespaces: map[string]cache.Config{
					operatorconfig.OCPClusterMonitoringNamespace: {
						FieldSelector: fields.OneTermEqualSelector("metadata.name", operatorconfig.OCPClusterMonitoringConfigMapName),
					},
					operatorconfig.OCPUserWorkloadMonitoringNamespace: {
						FieldSelector: fields.OneTermEqualSelector("metadata.name", operatorconfig.OCPUserWorkloadMonitoringConfigMap),
					},
				},
			},
			&corev1.Secret{}: {
				Namespaces: map[string]cache.Config{
					operatorconfig.OCPClusterMonitoringNamespace:      {},
					operatorconfig.OCPUserWorkloadMonitoringNamespace: {},
				},
			},
		},
	}
}
