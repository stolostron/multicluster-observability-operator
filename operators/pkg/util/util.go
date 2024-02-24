// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	mco_config "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	appv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Remove is used to remove string from a string array
func Remove(list []string, s string) []string {
	result := []string{}
	for _, v := range list {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}

// GetAnnotation returns the annotation value for a given key, or an empty string if not set
func GetAnnotation(annotations map[string]string, key string) string {
	if annotations == nil {
		return ""
	}
	return annotations[key]
}

// GeneratePassword returns a base64 encoded securely random bytes.
func GeneratePassword(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b), err
}

// ProxyEnvVarsAreSet ...
// OLM handles these environment variables as a unit;
// if at least one of them is set, all three are considered overridden
// and the cluster-wide defaults are not used for the deployments of the subscribed Operator.
// https://docs.openshift.com/container-platform/4.6/operators/admin/olm-configuring-proxy-support.html
func ProxyEnvVarsAreSet() bool {
	if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" || os.Getenv("NO_PROXY") != "" {
		return true
	}
	return false
}

func RemoveDuplicates(elements []string) []string {
	// Use map to record duplicates as we find them.
	encountered := map[string]struct{}{}
	result := []string{}

	for _, v := range elements {
		if _, found := encountered[v]; found {
			continue
		}
		encountered[v] = struct{}{}
		result = append(result, v)
	}
	// Return the new slice.
	return result
}

func RegisterDebugEndpoint(register func(string, http.Handler) error) error {
	err := register("/debug/", http.Handler(http.DefaultServeMux))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/", http.HandlerFunc(pprof.Index))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/block", http.Handler(pprof.Handler("block")))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/symobol", http.HandlerFunc(pprof.Symbol))
	if err != nil {
		return err
	}
	err = register("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	if err != nil {
		return err
	}

	return nil
}

func UpdateDeployLabel(c client.Client, dName, namespace, label string) error {
	dep := &appv1.Deployment{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      dName,
		Namespace: namespace,
	}, dep)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err, "Failed to check the deployment", "name", dName)
		}
		return err
	}
	if dep.Status.ReadyReplicas != 0 {
		dep.Spec.Template.ObjectMeta.Labels[label] = time.Now().Format("2006-1-2.1504")
		err = c.Update(context.TODO(), dep)
		if err != nil {
			log.Error(err, "Failed to update the deployment", "name", dName)
			return err
		} else {
			log.Info("Update deployment restart label", "name", dName)
		}
	}
	return nil
}

func GetResourceRequirementsforHubMetricsCollector(c client.Client) *corev1.ResourceRequirements {
	mco := &mcov1beta2.MultiClusterObservability{}
	err := c.Get(context.TODO(),
		types.NamespacedName{
			Name: mco_config.GetMonitoringCRName(),
		}, mco)
	if err != nil {
		log.Error(err, "Failed to get mco")
		return nil
	}
	return mco_config.GetOBAResources(mco.Spec.ObservabilityAddonSpec)
}
