// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

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

// BuildCertManagerOperatorResources returns the Namespace, OperatorGroup and Subscription
// objects required to install the cert-manager Operator for Red Hat OpenShift via OLM on the
// hub cluster.
//
// OperatorGroup/Subscription are built as unstructured objects (rather than importing
// github.com/operator-framework/api) to avoid adding a new dependency to this operator for
// three object kinds.
func BuildCertManagerOperatorResources() []client.Object {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: mcoconfig.CertManagerOperatorNamespace,
		},
	}

	// No spec.targetNamespaces/spec.selector: this makes it a global OperatorGroup (AllNamespaces
	// mode), which is the mode Red Hat's cert-manager Operator recommends from v1.15 onward.
	og := &unstructured.Unstructured{}
	og.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1",
		Kind:    "OperatorGroup",
	})
	og.SetName(mcoconfig.CertManagerOperatorPackageName)
	og.SetNamespace(mcoconfig.CertManagerOperatorNamespace)

	sub := &unstructured.Unstructured{}
	sub.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "Subscription",
	})
	sub.SetName(mcoconfig.CertManagerOperatorPackageName)
	sub.SetNamespace(mcoconfig.CertManagerOperatorNamespace)
	sub.Object["spec"] = map[string]any{
		// channel is deliberately omitted: it's an optional field on the Subscription CRD, and
		// leaving it unset tells OLM to track the package's current default channel, so this
		// doesn't need to be bumped as the operator ships new channels (see loki_operator.go for
		// the same reasoning).
		"name":                mcoconfig.CertManagerOperatorPackageName,
		"source":              mcoconfig.CertManagerOperatorCatalogSource,
		"sourceNamespace":     mcoconfig.CertManagerOperatorCatalogSourceNamespace,
		"installPlanApproval": "Automatic",
	}

	return []client.Object{ns, og, sub}
}

// EnsureCertManagerOperatorInstalled creates the cert-manager Operator's Namespace,
// OperatorGroup and Subscription if they don't already exist. It never updates an existing
// object, so it won't fight a manual install or an in-progress OLM upgrade.
func EnsureCertManagerOperatorInstalled(ctx context.Context, c client.Client) error {
	for _, obj := range BuildCertManagerOperatorResources() {
		if err := c.Create(ctx, obj); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s %s/%s: %w", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName(), err)
		}
	}
	return nil
}
