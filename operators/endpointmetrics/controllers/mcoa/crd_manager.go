// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"embed"
	"fmt"
	"path/filepath"
	"slices"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

//go:embed crds/*.yaml
var embeddedCRDs embed.FS

const (
	ManagedByLabelKey   = "app.kubernetes.io/managed-by"
	ManagedByLabelValue = "mcoa-endpoint-operator"
)

var crdLog = ctrl.Log.WithName("crd-manager")

// managedCRDNames is the set of CRD names we own, derived from the embedded files at
// init time. Used to filter watch events without re-reading the embedded FS on every call.
var managedCRDNames map[string]struct{}

func init() {
	crds, err := loadEmbeddedCRDs()
	if err != nil {
		// Embedded files are compiled into the binary; a parse failure means a corrupt build.
		panic(fmt.Sprintf("failed to initialize managed CRD name set: %v", err))
	}
	managedCRDNames = make(map[string]struct{}, len(crds))
	for _, crd := range crds {
		managedCRDNames[crd.GetName()] = struct{}{}
	}
}

// isManagedCRDName reports whether name belongs to an OBO CRD owned by this operator.
func isManagedCRDName(name string) bool {
	_, ok := managedCRDNames[name]
	return ok
}

// GetManagedCRDNames returns a sorted slice of all CustomResourceDefinition names managed by MCOA,
// derived dynamically from the embedded files.
func GetManagedCRDNames() []string {
	names := make([]string, 0, len(managedCRDNames))
	for name := range managedCRDNames {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// loadEmbeddedCRDs reads and unmarshals the OBO CRDs from the embedded filesystem.
func loadEmbeddedCRDs() ([]*unstructured.Unstructured, error) {
	entries, err := embeddedCRDs.ReadDir("crds")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded crds directory: %w", err)
	}

	var crds []*unstructured.Unstructured
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		data, err := embeddedCRDs.ReadFile(filepath.Join("crds", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read embedded file %s: %w", entry.Name(), err)
		}

		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(data, obj); err != nil {
			return nil, fmt.Errorf("failed to unmarshal yaml %s: %w", entry.Name(), err)
		}

		// Apply our tracking label unconditionally in memory.
		// This ensures that any Server-Side Apply call (initial or upgrade) preserves our ownership.
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[ManagedByLabelKey] = ManagedByLabelValue
		obj.SetLabels(labels)

		crds = append(crds, obj)
	}
	return crds, nil
}

// isManagedByUs checks if the CRD possesses our ownership tracking label.
func isManagedByUs(obj *unstructured.Unstructured) bool {
	labels := obj.GetLabels()
	return labels != nil && labels[ManagedByLabelKey] == ManagedByLabelValue
}

// DeployCRDs checks if the monitoring.rhobs CRDs exist.
// If they do not exist, it applies them using Server-Side Apply (SSA) with our label.
// If they exist and are managed by us, it upgrades their schema using Server-Side Apply with ForceOwnership.
func DeployCRDs(ctx context.Context, c client.Client) error {
	crds, err := loadEmbeddedCRDs()
	if err != nil {
		return err
	}

	for _, obj := range crds {
		// Check if CRD already exists
		found := &unstructured.Unstructured{}
		found.SetGroupVersionKind(obj.GroupVersionKind())
		err = c.Get(ctx, types.NamespacedName{Name: obj.GetName()}, found)
		if err != nil {
			if errors.IsNotFound(err) {
				// Server-side apply without forcing ownership
				if err := c.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), client.FieldOwner(ManagedByLabelValue)); err != nil {
					return fmt.Errorf("failed to server-side apply CRD %s: %w", obj.GetName(), err)
				}
			} else {
				return fmt.Errorf("failed to check existence of CRD %s: %w", obj.GetName(), err)
			}
		} else {
			// CRD exists. Check if it is managed by us before attempting an upgrade.
			if isManagedByUs(found) {
				// Unconditionally apply with ForceOwnership to progress schemas safely
				if err := c.Apply(ctx, client.ApplyConfigurationFromUnstructured(obj), client.FieldOwner(ManagedByLabelValue), client.ForceOwnership); err != nil {
					return fmt.Errorf("failed to update managed CRD %s via server-side apply: %w", obj.GetName(), err)
				}
			} else {
				crdLog.Info("Skipped deploying CRD because it is managed by another actor", "name", obj.GetName())
			}
		}
	}

	return nil
}

// CleanUpCRDs retrieves the monitoring.rhobs CRDs and deletes them
// if they have the mcoa-endpoint-operator management label.
func CleanUpCRDs(ctx context.Context, c client.Client) error {
	crds, err := loadEmbeddedCRDs()
	if err != nil {
		return err
	}

	for _, obj := range crds {
		found := &unstructured.Unstructured{}
		found.SetGroupVersionKind(obj.GroupVersionKind())
		err = c.Get(ctx, types.NamespacedName{Name: obj.GetName()}, found)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("failed to fetch CRD %s for cleanup: %w", obj.GetName(), err)
		}

		if isManagedByUs(found) {
			if err := c.Delete(ctx, found); err != nil {
				return fmt.Errorf("failed to delete managed CRD %s: %w", found.GetName(), err)
			}
		}
	}

	return nil
}
