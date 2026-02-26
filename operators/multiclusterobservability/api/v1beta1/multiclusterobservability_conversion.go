// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package v1beta1

/*
For imports, we'll need the controller-runtime
[`conversion`](https://godoc.org/sigs.k8s.io/controller-runtime/pkg/conversion)
package, plus the API version for our hub type (v1beta2), and finally some of the
standard packages.
*/
import (
	observabilityv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// +kubebuilder:docs-gen:collapse=Imports

/*
Our "spoke" versions need to implement the
[`Convertible`](https://godoc.org/sigs.k8s.io/controller-runtime/pkg/conversion#Convertible)
interface.  Namely, they'll need `ConvertTo` and `ConvertFrom` methods to convert to/from
the hub version.
*/

/*
ConvertTo is expected to modify its argument to contain the converted object.
Most of the conversion is straightforward copying, except for converting our changed field.
*/
// ConvertTo converts this MultiClusterObservability to the Hub version (v1beta2).
func (m *MultiClusterObservability) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*observabilityv1beta2.MultiClusterObservability)

	// TODO(morvencao)?: convert the AvailabilityConfig field
	// availabilityConfig := src.Spec.AvailabilityConfig

	dst.Spec.StorageConfig = &observabilityv1beta2.StorageConfig{
		MetricObjectStorage: m.Spec.StorageConfig.MetricObjectStorage,
		StorageClass:        m.Spec.StorageConfig.StatefulSetStorageClass,
		// How to convert the current storage size to new one?
		AlertmanagerStorageSize: m.Spec.StorageConfig.StatefulSetSize,
		RuleStorageSize:         m.Spec.StorageConfig.StatefulSetSize,
		StoreStorageSize:        m.Spec.StorageConfig.StatefulSetSize,
		CompactStorageSize:      m.Spec.StorageConfig.StatefulSetSize,
		ReceiveStorageSize:      m.Spec.StorageConfig.StatefulSetSize,
	}

	dst.Spec.AdvancedConfig = &observabilityv1beta2.AdvancedConfig{
		RetentionConfig: &observabilityv1beta2.RetentionConfig{
			RetentionResolutionRaw: m.Spec.RetentionResolutionRaw,
			RetentionResolution5m:  m.Spec.RetentionResolution5m,
			RetentionResolution1h:  m.Spec.RetentionResolution1h,
		},
	}

	dst.Spec.EnableDownsampling = m.Spec.EnableDownSampling

	/*
		The rest of the conversion is pretty rote.
	*/
	// ObjectMeta
	dst.ObjectMeta = m.ObjectMeta

	// Spec
	dst.Spec.ImagePullPolicy = m.Spec.ImagePullPolicy
	dst.Spec.ImagePullSecret = m.Spec.ImagePullSecret
	dst.Spec.NodeSelector = m.Spec.NodeSelector
	dst.Spec.Tolerations = m.Spec.Tolerations
	dst.Spec.ObservabilityAddonSpec = m.Spec.ObservabilityAddonSpec

	// Status
	dst.Status.Conditions = m.Status.Conditions

	// +kubebuilder:docs-gen:collapse=rote conversion
	return nil
}

/*
ConvertFrom is expected to modify its receiver to contain the converted object.
Most of the conversion is straightforward copying, except for converting our changed field.
*/

// ConvertFrom converts from the Hub version (observabilityv1beta2) to this version.
func (m *MultiClusterObservability) ConvertFrom(srcRaw conversion.Hub) error {
	hubSrc := srcRaw.(*observabilityv1beta2.MultiClusterObservability)

	// TODO(morvencao): convert the AvailabilityConfig field
	// src.Spec.AvailabilityConfig =

	if hubSrc.Spec.AdvancedConfig != nil && hubSrc.Spec.AdvancedConfig.RetentionConfig != nil {
		m.Spec.RetentionResolutionRaw = hubSrc.Spec.AdvancedConfig.RetentionConfig.RetentionResolutionRaw
		m.Spec.RetentionResolution5m = hubSrc.Spec.AdvancedConfig.RetentionConfig.RetentionResolution5m
		m.Spec.RetentionResolution1h = hubSrc.Spec.AdvancedConfig.RetentionConfig.RetentionResolution1h
	}

	m.Spec.StorageConfig = &StorageConfigObject{
		MetricObjectStorage:     hubSrc.Spec.StorageConfig.MetricObjectStorage,
		StatefulSetStorageClass: hubSrc.Spec.StorageConfig.StorageClass,
		// How to convert the new storage size to old one?
		// StatefulSetSize =
	}

	m.Spec.EnableDownSampling = hubSrc.Spec.EnableDownsampling

	/*
		The rest of the conversion is pretty rote.
	*/
	// ObjectMeta
	m.ObjectMeta = hubSrc.ObjectMeta

	// Spec
	m.Spec.ImagePullPolicy = hubSrc.Spec.ImagePullPolicy
	m.Spec.ImagePullSecret = hubSrc.Spec.ImagePullSecret
	m.Spec.NodeSelector = hubSrc.Spec.NodeSelector
	m.Spec.Tolerations = hubSrc.Spec.Tolerations
	m.Spec.ObservabilityAddonSpec = hubSrc.Spec.ObservabilityAddonSpec

	// Status
	m.Status.Conditions = hubSrc.Status.Conditions

	// +kubebuilder:docs-gen:collapse=rote conversion
	return nil
}
