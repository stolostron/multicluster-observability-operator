// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package deploying

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
)

var log = logf.Log.WithName("deploying")

type deployerFn func(*unstructured.Unstructured, *unstructured.Unstructured) error

// Deployer is used create or update the resources.
type Deployer struct {
	client      client.Client
	deployerFns map[string]deployerFn
}

// NewDeployer inits the deployer.
func NewDeployer(client client.Client) *Deployer {
	deployer := &Deployer{client: client}
	deployer.deployerFns = map[string]deployerFn{
		"Deployment":               deployer.updateDeployment,
		"StatefulSet":              deployer.updateStatefulSet,
		"Service":                  deployer.updateService,
		"ConfigMap":                deployer.updateConfigMap,
		"Secret":                   deployer.updateSecret,
		"ClusterRole":              deployer.updateClusterRole,
		"ClusterRoleBinding":       deployer.updateClusterRoleBinding,
		"CustomResourceDefinition": deployer.updateCRD,
		"Prometheus":               deployer.updatePrometheus,
		"PrometheusRule":           deployer.updatePrometheusRule,
		"Ingress":                  deployer.updateIngress,
	}
	return deployer
}

// Deploy is used to create or update the resources.
func (d *Deployer) Deploy(obj *unstructured.Unstructured) error {
	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(obj.GroupVersionKind())
	err := d.client.Get(
		context.TODO(),
		types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()},
		found,
	)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Create", "Kind", obj.GroupVersionKind(), "Name", obj.GetName())
			return d.client.Create(context.TODO(), obj)
		}
		return err
	}

	// if resource has annotation skip-creation-if-exist: true, don't update it to keep customized changes from users
	metadata, ok := obj.Object["metadata"].(map[string]interface{})
	if ok {
		annotations, ok := metadata["annotations"].(map[string]interface{})
		if ok && annotations != nil && annotations[config.AnnotationSkipCreation] != nil {
			if strings.ToLower(annotations[config.AnnotationSkipCreation].(string)) == "true" {
				log.Info("Skip creation", "Kind", obj.GroupVersionKind(), "Name", obj.GetName())
				return nil
			}
		}
	}

	deployerFn, ok := d.deployerFns[found.GetKind()]
	if ok {
		return deployerFn(obj, found)
	} else {
		log.Info("deployerFn not found", "kind", found.GetKind())
	}
	return nil
}

func (d *Deployer) updateDeployment(desiredObj, runtimeObj *unstructured.Unstructured) error {
	runtimeJSON, _ := runtimeObj.MarshalJSON()
	runtimeDepoly := &appsv1.Deployment{}
	err := json.Unmarshal(runtimeJSON, runtimeDepoly)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal runtime Deployment %s", runtimeObj.GetName()))
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredDepoly := &appsv1.Deployment{}
	err = json.Unmarshal(desiredJSON, desiredDepoly)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal Deployment %s", runtimeObj.GetName()))
	}

	if !apiequality.Semantic.DeepDerivative(desiredDepoly.Spec, runtimeDepoly.Spec) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(context.TODO(), desiredDepoly)
	}

	return nil
}

func (d *Deployer) updateStatefulSet(desiredObj, runtimeObj *unstructured.Unstructured) error {
	runtimeJSON, _ := runtimeObj.MarshalJSON()
	runtimeDepoly := &appsv1.StatefulSet{}
	err := json.Unmarshal(runtimeJSON, runtimeDepoly)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal runtime StatefulSet %s", runtimeObj.GetName()))
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredDepoly := &appsv1.StatefulSet{}
	err = json.Unmarshal(desiredJSON, desiredDepoly)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal StatefulSet %s", runtimeObj.GetName()))
	}

	if !apiequality.Semantic.DeepDerivative(desiredDepoly.Spec.Template, runtimeDepoly.Spec.Template) ||
		!apiequality.Semantic.DeepDerivative(desiredDepoly.Spec.Replicas, runtimeDepoly.Spec.Replicas) {
		logUpdateInfo(runtimeObj)
		runtimeDepoly.Spec.Replicas = desiredDepoly.Spec.Replicas
		runtimeDepoly.Spec.Template = desiredDepoly.Spec.Template
		return d.client.Update(context.TODO(), runtimeDepoly)
	}

	return nil
}

func (d *Deployer) updateService(desiredObj, runtimeObj *unstructured.Unstructured) error {
	runtimeJSON, _ := runtimeObj.MarshalJSON()
	runtimeService := &corev1.Service{}
	err := json.Unmarshal(runtimeJSON, runtimeService)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal runtime Service %s", runtimeObj.GetName()))
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredService := &corev1.Service{}
	err = json.Unmarshal(desiredJSON, desiredService)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal Service %s", runtimeObj.GetName()))
	}

	if !apiequality.Semantic.DeepDerivative(desiredService.Spec, runtimeService.Spec) {
		desiredService.ObjectMeta.ResourceVersion = runtimeService.ObjectMeta.ResourceVersion
		desiredService.Spec.ClusterIP = runtimeService.Spec.ClusterIP
		logUpdateInfo(runtimeObj)
		return d.client.Update(context.TODO(), desiredService)
	}

	return nil
}

func (d *Deployer) updateConfigMap(desiredObj, runtimeObj *unstructured.Unstructured) error {
	runtimeJSON, _ := runtimeObj.MarshalJSON()
	runtimeConfigMap := &corev1.ConfigMap{}
	err := json.Unmarshal(runtimeJSON, runtimeConfigMap)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal runtime ConfigMap %s", runtimeObj.GetName()))
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredConfigMap := &corev1.ConfigMap{}
	err = json.Unmarshal(desiredJSON, desiredConfigMap)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal ConfigMap %s", runtimeObj.GetName()))
	}

	if !apiequality.Semantic.DeepDerivative(desiredConfigMap.Data, runtimeConfigMap.Data) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(context.TODO(), desiredConfigMap)
	}

	return nil
}

func (d *Deployer) updateSecret(desiredObj, runtimeObj *unstructured.Unstructured) error {
	runtimeJSON, _ := runtimeObj.MarshalJSON()
	runtimeSecret := &corev1.Secret{}
	err := json.Unmarshal(runtimeJSON, runtimeSecret)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal runtime Secret %s", runtimeObj.GetName()))
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredSecret := &corev1.Secret{}
	err = json.Unmarshal(desiredJSON, desiredSecret)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal desired Secret %s", desiredObj.GetName()))
	}

	if desiredSecret.Data == nil ||
		!apiequality.Semantic.DeepDerivative(desiredSecret.Data, runtimeSecret.Data) {
		logUpdateInfo(desiredObj)
		return d.client.Update(context.TODO(), desiredSecret)
	}
	return nil
}

func (d *Deployer) updateClusterRole(desiredObj, runtimeObj *unstructured.Unstructured) error {
	runtimeJSON, _ := runtimeObj.MarshalJSON()
	runtimeClusterRole := &rbacv1.ClusterRole{}
	err := json.Unmarshal(runtimeJSON, runtimeClusterRole)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal runtime ClusterRole %s", runtimeObj.GetName()))
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredClusterRole := &rbacv1.ClusterRole{}
	err = json.Unmarshal(desiredJSON, desiredClusterRole)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal desired ClusterRole %s", desiredObj.GetName()))
	}

	if !apiequality.Semantic.DeepDerivative(desiredClusterRole.Rules, runtimeClusterRole.Rules) ||
		!apiequality.Semantic.DeepDerivative(desiredClusterRole.AggregationRule, runtimeClusterRole.AggregationRule) {
		logUpdateInfo(desiredObj)
		return d.client.Update(context.TODO(), desiredClusterRole)
	}
	return nil
}

func (d *Deployer) updateClusterRoleBinding(desiredObj, runtimeObj *unstructured.Unstructured) error {
	runtimeJSON, _ := runtimeObj.MarshalJSON()
	runtimeClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	err := json.Unmarshal(runtimeJSON, runtimeClusterRoleBinding)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal runtime ClusterRoleBinding %s", runtimeObj.GetName()))
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	err = json.Unmarshal(desiredJSON, desiredClusterRoleBinding)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal desired ClusterRoleBinding %s", desiredObj.GetName()))
	}

	if !apiequality.Semantic.DeepDerivative(desiredClusterRoleBinding.Subjects, runtimeClusterRoleBinding.Subjects) ||
		!apiequality.Semantic.DeepDerivative(desiredClusterRoleBinding.RoleRef, runtimeClusterRoleBinding.RoleRef) {
		logUpdateInfo(desiredObj)
		return d.client.Update(context.TODO(), desiredClusterRoleBinding)
	}
	return nil
}

func (d *Deployer) updateCRD(desiredObj, runtimeObj *unstructured.Unstructured) error {
	runtimeJSON, _ := runtimeObj.MarshalJSON()
	runtimeCRD := &apiextensionsv1.CustomResourceDefinition{}
	err := json.Unmarshal(runtimeJSON, runtimeCRD)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal runtime CRD %s", runtimeObj.GetName()))
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredCRD := &apiextensionsv1.CustomResourceDefinition{}
	err = json.Unmarshal(desiredJSON, desiredCRD)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal CRD %s", runtimeObj.GetName()))
	}
	desiredCRD.ObjectMeta.ResourceVersion = runtimeCRD.ObjectMeta.ResourceVersion

	if !apiequality.Semantic.DeepDerivative(desiredCRD.Spec, runtimeCRD.Spec) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(context.TODO(), desiredCRD)
	}

	return nil
}

func (d *Deployer) updatePrometheus(desiredObj, runtimeObj *unstructured.Unstructured) error {
	runtimeJSON, _ := runtimeObj.MarshalJSON()
	runtimePrometheus := &prometheusv1.Prometheus{}
	err := json.Unmarshal(runtimeJSON, runtimePrometheus)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal runtime Prometheus %s", runtimeObj.GetName()))
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredPrometheus := &prometheusv1.Prometheus{}
	err = json.Unmarshal(desiredJSON, desiredPrometheus)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal Prometheus %s", runtimeObj.GetName()))
	}

	// On GKE clusters, it was observed that the runtime object was not in sync with the object attributes
	// seen via kube client. There may be an issue with caching inside the operator that may need to be
	// investigated. For now, if the Prometheus attributes are not picked up by operator, by performing the
	// the two operations, the object will be correctly regenetated.
	// 1. delete Prometheus object
	// 2. delete endpoint operator pod

	// inherit resource version if not specified
	if desiredPrometheus.ResourceVersion != runtimePrometheus.ResourceVersion {
		desiredPrometheus.ResourceVersion = runtimePrometheus.ResourceVersion
	}

	if runtimePrometheus.Spec.AdditionalAlertManagerConfigs != nil {
		log.Info("Runtime Prometheus: AdditionalAlertManagerConfig", "object",
			fmt.Sprintf("%v", runtimePrometheus.Spec.AdditionalAlertManagerConfigs))
	} else {
		log.Info("Runtime Prometheus: AdditionalAlertManagerConfig is null")
	}

	if desiredPrometheus.Spec.AdditionalAlertManagerConfigs != nil {
		log.Info("Desired Prometheus: AdditionalAlertManagerConfig", "object:",
			fmt.Sprintf("%v", desiredPrometheus.Spec.AdditionalAlertManagerConfigs))
	} else {
		log.Info("Desired Prometheus: AdditionalAlertManagerConfig is null")
	}

	if !apiequality.Semantic.DeepDerivative(desiredPrometheus.Spec, runtimePrometheus.Spec) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(context.TODO(), desiredPrometheus)
	} else {
		log.Info("Runtime Prometheus and Desired Prometheus are semantically equal!")
	}
	return nil
}

func (d *Deployer) updatePrometheusRule(desiredObj, runtimeObj *unstructured.Unstructured) error {
	runtimeJSON, _ := runtimeObj.MarshalJSON()
	runtimePrometheusRule := &prometheusv1.PrometheusRule{}
	err := json.Unmarshal(runtimeJSON, runtimePrometheusRule)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal runtime PrometheusRule  %s", runtimeObj.GetName()))
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredPrometheusRule := &prometheusv1.PrometheusRule{}
	err = json.Unmarshal(desiredJSON, desiredPrometheusRule)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal PrometheusRule  %s", runtimeObj.GetName()))
	}

	if !apiequality.Semantic.DeepDerivative(desiredPrometheusRule.Spec, runtimePrometheusRule.Spec) {
		logUpdateInfo(runtimeObj)
		if desiredPrometheusRule.ResourceVersion != runtimePrometheusRule.ResourceVersion {
			desiredPrometheusRule.ResourceVersion = runtimePrometheusRule.ResourceVersion
		}

		return d.client.Update(context.TODO(), desiredPrometheusRule)
	}
	return nil
}

func (d *Deployer) updateIngress(desiredObj, runtimeObj *unstructured.Unstructured) error {
	runtimeJSON, _ := runtimeObj.MarshalJSON()
	runtimeIngress := &networkingv1.Ingress{}
	err := json.Unmarshal(runtimeJSON, runtimeIngress)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal runtime Ingress %s", runtimeObj.GetName()))
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredIngress := &networkingv1.Ingress{}
	err = json.Unmarshal(desiredJSON, desiredIngress)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to Unmarshal Ingress %s", runtimeObj.GetName()))
	}

	if !apiequality.Semantic.DeepDerivative(desiredIngress.Spec, runtimeIngress.Spec) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(context.TODO(), desiredIngress)
	}

	return nil
}

func logUpdateInfo(obj *unstructured.Unstructured) {
	log.Info("Update", "kind", obj.GroupVersionKind().Kind, "kindVersion", obj.GroupVersionKind().Version, "name", obj.GetName())
}
