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
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
)

var log = logf.Log.WithName("deploying")

type deployerFn func(context.Context, *unstructured.Unstructured, *unstructured.Unstructured) error

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
		"Role":                     deployer.updateRole,
		"RoleBinding":              deployer.updateRoleBinding,
		"ServiceAccount":           deployer.updateServiceAccount,
		"DaemonSet":                deployer.updateDaemonSet,
		"ServiceMonitor":           deployer.updateServiceMonitor,
		"AddOnDeploymentConfig":    deployer.updateAddOnDeploymentConfig,
		"ClusterManagementAddOn":   deployer.updateClusterManagementAddOn,
	}
	return deployer
}

// Deploy is used to create or update the resources.
func (d *Deployer) Deploy(ctx context.Context, obj *unstructured.Unstructured) error {
	// Create the resource if it doesn't exist
	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(obj.GroupVersionKind())
	err := d.client.Get(
		ctx,
		types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()},
		found,
	)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Create", "Kind", obj.GroupVersionKind(), "Name", obj.GetName())
			return d.client.Create(ctx, obj)
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

	// The resource exists, update it
	deployerFn, ok := d.deployerFns[found.GetKind()]
	if ok {
		return deployerFn(ctx, obj, found)
	} else {
		log.Info("deployerFn not found", "kind", found.GetKind())
	}
	return nil
}

func (d *Deployer) updateDeployment(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredDeploy, runtimeDepoly, err := unstructuredPairToTyped[appsv1.Deployment](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredDeploy.Spec, runtimeDepoly.Spec) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(ctx, desiredDeploy)
	}

	return nil
}

func (d *Deployer) updateStatefulSet(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredDepoly, runtimeDepoly, err := unstructuredPairToTyped[appsv1.StatefulSet](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredDepoly.Spec.Template, runtimeDepoly.Spec.Template) ||
		!apiequality.Semantic.DeepDerivative(desiredDepoly.Spec.Replicas, runtimeDepoly.Spec.Replicas) {
		logUpdateInfo(runtimeObj)
		runtimeDepoly.Spec.Replicas = desiredDepoly.Spec.Replicas
		runtimeDepoly.Spec.Template = desiredDepoly.Spec.Template
		return d.client.Update(ctx, runtimeDepoly)
	}

	return nil
}

func (d *Deployer) updateService(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredService, runtimeService, err := unstructuredPairToTyped[corev1.Service](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredService.Spec, runtimeService.Spec) {
		desiredService.ObjectMeta.ResourceVersion = runtimeService.ObjectMeta.ResourceVersion
		desiredService.Spec.ClusterIP = runtimeService.Spec.ClusterIP
		logUpdateInfo(runtimeObj)
		return d.client.Update(ctx, desiredService)
	}

	return nil
}

func (d *Deployer) updateConfigMap(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredConfigMap, runtimeConfigMap, err := unstructuredPairToTyped[corev1.ConfigMap](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredConfigMap.Data, runtimeConfigMap.Data) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(ctx, desiredConfigMap)
	}

	return nil
}

func (d *Deployer) updateSecret(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredSecret, runtimeSecret, err := unstructuredPairToTyped[corev1.Secret](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if desiredSecret.Data == nil ||
		!apiequality.Semantic.DeepDerivative(desiredSecret.Data, runtimeSecret.Data) {
		logUpdateInfo(desiredObj)
		return d.client.Update(ctx, desiredSecret)
	}
	return nil
}

func (d *Deployer) updateClusterRole(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredClusterRole, runtimeClusterRole, err := unstructuredPairToTyped[rbacv1.ClusterRole](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredClusterRole.Rules, runtimeClusterRole.Rules) ||
		!apiequality.Semantic.DeepDerivative(desiredClusterRole.AggregationRule, runtimeClusterRole.AggregationRule) {
		logUpdateInfo(desiredObj)
		return d.client.Update(ctx, desiredClusterRole)
	}
	return nil
}

func (d *Deployer) updateClusterRoleBinding(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredClusterRoleBinding, runtimeClusterRoleBinding, err := unstructuredPairToTyped[rbacv1.ClusterRoleBinding](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredClusterRoleBinding.Subjects, runtimeClusterRoleBinding.Subjects) ||
		!apiequality.Semantic.DeepDerivative(desiredClusterRoleBinding.RoleRef, runtimeClusterRoleBinding.RoleRef) {
		logUpdateInfo(desiredObj)
		return d.client.Update(ctx, desiredClusterRoleBinding)
	}
	return nil
}

func (d *Deployer) updateCRD(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredCRD, runtimeCRD, err := unstructuredPairToTyped[apiextensionsv1.CustomResourceDefinition](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	desiredCRD.ObjectMeta.ResourceVersion = runtimeCRD.ObjectMeta.ResourceVersion

	if !apiequality.Semantic.DeepDerivative(desiredCRD.Spec, runtimeCRD.Spec) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(ctx, desiredCRD)
	}

	return nil
}

func (d *Deployer) updatePrometheus(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredPrometheus, runtimePrometheus, err := unstructuredPairToTyped[prometheusv1.Prometheus](desiredObj, runtimeObj)
	if err != nil {
		return err
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
		return d.client.Update(ctx, desiredPrometheus)
	} else {
		log.Info("Runtime Prometheus and Desired Prometheus are semantically equal!")
	}
	return nil
}

func (d *Deployer) updatePrometheusRule(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredPrometheusRule, runtimePrometheusRule, err := unstructuredPairToTyped[prometheusv1.PrometheusRule](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredPrometheusRule.Spec, runtimePrometheusRule.Spec) {
		logUpdateInfo(runtimeObj)
		if desiredPrometheusRule.ResourceVersion != runtimePrometheusRule.ResourceVersion {
			desiredPrometheusRule.ResourceVersion = runtimePrometheusRule.ResourceVersion
		}

		return d.client.Update(ctx, desiredPrometheusRule)
	}
	return nil
}

func (d *Deployer) updateIngress(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredIngress, runtimeIngress, err := unstructuredPairToTyped[networkingv1.Ingress](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredIngress.Spec, runtimeIngress.Spec) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(ctx, desiredIngress)
	}

	return nil
}

func (d *Deployer) updateRole(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredRole, runtimeRole, err := unstructuredPairToTyped[rbacv1.Role](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredRole.Rules, runtimeRole.Rules) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(ctx, desiredRole)
	}

	return nil
}

func (d *Deployer) updateRoleBinding(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredRoleBinding, runtimeRoleBinding, err := unstructuredPairToTyped[rbacv1.RoleBinding](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredRoleBinding.Subjects, runtimeRoleBinding.Subjects) ||
		!apiequality.Semantic.DeepDerivative(desiredRoleBinding.RoleRef, runtimeRoleBinding.RoleRef) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(ctx, desiredRoleBinding)
	}

	return nil
}

func (d *Deployer) updateServiceAccount(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredServiceAccount, runtimeServiceAccount, err := unstructuredPairToTyped[corev1.ServiceAccount](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredServiceAccount.ImagePullSecrets, runtimeServiceAccount.ImagePullSecrets) ||
		!apiequality.Semantic.DeepDerivative(desiredServiceAccount.Secrets, runtimeServiceAccount.Secrets) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(ctx, desiredServiceAccount)
	}

	return nil
}

func (d *Deployer) updateDaemonSet(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredDaemonSet, runtimeDaemonSet, err := unstructuredPairToTyped[appsv1.DaemonSet](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredDaemonSet.Spec, runtimeDaemonSet.Spec) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(ctx, desiredDaemonSet)
	}

	return nil
}

func (d *Deployer) updateServiceMonitor(ctx context.Context, desiredObj, runtimeObj *unstructured.Unstructured) error {
	desiredServiceMonitor, runtimeServiceMonitor, err := unstructuredPairToTyped[prometheusv1.ServiceMonitor](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredServiceMonitor.Spec, runtimeServiceMonitor.Spec) {
		logUpdateInfo(runtimeObj)
		return d.client.Update(ctx, desiredServiceMonitor)
	}

	return nil
}

func (d *Deployer) updateAddOnDeploymentConfig(
	ctx context.Context,
	desiredObj, runtimeObj *unstructured.Unstructured,
) error {
	desiredAODC, runtimeAODC, err := unstructuredPairToTyped[addonv1alpha1.AddOnDeploymentConfig](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepEqual(desiredAODC.Spec, runtimeAODC.Spec) {
		logUpdateInfo(runtimeObj)
		if desiredAODC.ResourceVersion != runtimeAODC.ResourceVersion {
			desiredAODC.ResourceVersion = runtimeAODC.ResourceVersion
		}
		return d.client.Update(ctx, desiredAODC)
	}

	return nil
}

func (d *Deployer) updateClusterManagementAddOn(
	ctx context.Context,
	desiredObj, runtimeObj *unstructured.Unstructured,
) error {
	desiredCMAO, runtimeCMAO, err := unstructuredPairToTyped[addonv1alpha1.ClusterManagementAddOn](desiredObj, runtimeObj)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepEqual(desiredCMAO.Spec, runtimeCMAO.Spec) {
		logUpdateInfo(runtimeObj)
		if desiredCMAO.ResourceVersion != runtimeCMAO.ResourceVersion {
			desiredCMAO.ResourceVersion = runtimeCMAO.ResourceVersion
		}
		return d.client.Update(ctx, desiredCMAO)
	}

	return nil
}

// unstructuredToType converts an unstructured.Unstructured object to a specified type.
// It marshals the object to JSON and then unmarshals it into the target type.
// The target parameter must be a pointer to the type T.
func unstructuredToType[T any](obj *unstructured.Unstructured, target T) error {
	jsonData, err := obj.MarshalJSON()
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonData, target)
}

// unstructuredPairToTyped converts a pair of unstructured.Unstructured objects to a specified type.
func unstructuredPairToTyped[T any](obja, objb *unstructured.Unstructured) (*T, *T, error) {
	a := new(T)
	if err := unstructuredToType(obja, a); err != nil {
		return nil, nil, fmt.Errorf("failed to convert obja %s/%s/%s: %w", obja.GetKind(), obja.GetNamespace(), obja.GetName(), err)
	}

	b := new(T)
	if err := unstructuredToType(objb, b); err != nil {
		return nil, nil, fmt.Errorf("failed to convert objb %s/%s/%s: %w", obja.GetKind(), obja.GetNamespace(), obja.GetName(), err)
	}

	return a, b, nil
}

func logUpdateInfo(obj *unstructured.Unstructured) {
	log.Info("Update", "kind", obj.GroupVersionKind().Kind, "kindVersion", obj.GroupVersionKind().Version, "name", obj.GetName())
}

func (d *Deployer) Undeploy(ctx context.Context, obj *unstructured.Unstructured) error {
	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(obj.GroupVersionKind())
	err := d.client.Get(
		ctx,
		types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()},
		found,
	)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return d.client.Delete(ctx, obj)
}
