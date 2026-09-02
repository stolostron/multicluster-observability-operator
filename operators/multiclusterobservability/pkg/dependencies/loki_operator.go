// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

// Package dependencies installs third-party operators that MCOA capabilities depend on but
// that MCO does not otherwise own or manage (e.g. Loki Operator, which provides the LokiStack
// CRD used as the default log store for platform log collection).
package dependencies

import (
	"context"
	"fmt"

	mcoconfig "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BuildLokiOperatorResources returns the Namespace, OperatorGroup and Subscription objects
// required to install Loki Operator via OLM on the hub cluster.
//
// OperatorGroup/Subscription are built as unstructured objects (rather than importing
// github.com/operator-framework/api) to avoid adding a new dependency to this operator for
// three object kinds.
func BuildLokiOperatorResources() []client.Object {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoconfig.LokiOperatorNamespace,
		},
	}

	og := &unstructured.Unstructured{}
	og.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1",
		Kind:    "OperatorGroup",
	})
	og.SetName(mcoconfig.LokiOperatorPackageName)
	og.SetNamespace(mcoconfig.LokiOperatorNamespace)

	sub := &unstructured.Unstructured{}
	sub.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "Subscription",
	})
	sub.SetName(mcoconfig.LokiOperatorPackageName)
	sub.SetNamespace(mcoconfig.LokiOperatorNamespace)
	sub.Object["spec"] = map[string]any{
		"channel":             mcoconfig.LokiOperatorChannel,
		"name":                mcoconfig.LokiOperatorPackageName,
		"source":              mcoconfig.LokiOperatorCatalogSource,
		"sourceNamespace":     mcoconfig.LokiOperatorCatalogSourceNamespace,
		"installPlanApproval": "Automatic",
	}

	return []client.Object{ns, og, sub}
}

// EnsureLokiOperatorInstalled creates the Loki Operator's Namespace, OperatorGroup and
// Subscription if they don't already exist. It never updates an existing object, so it won't
// fight a manual install or an in-progress OLM upgrade.
func EnsureLokiOperatorInstalled(ctx context.Context, c client.Client) error {
	for _, obj := range BuildLokiOperatorResources() {
		if err := c.Create(ctx, obj); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s %s/%s: %w", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName(), err)
		}
	}
	return nil
}
