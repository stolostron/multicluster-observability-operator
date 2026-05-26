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
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	observabilityv1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
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
func (src *MultiClusterObservability) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*observabilityv1beta2.MultiClusterObservability)

	// TODO(morvencao)?: convert the AvailabilityConfig field
	// availabilityConfig := src.Spec.AvailabilityConfig

	dst.Spec.StorageConfig = &observabilityv1beta2.StorageConfig{
		MetricObjectStorage: src.Spec.StorageConfig.MetricObjectStorage,
		StorageClass:        src.Spec.StorageConfig.StatefulSetStorageClass,
		// How to convert the current storage size to new one?
		AlertmanagerStorageSize: src.Spec.StorageConfig.StatefulSetSize,
		RuleStorageSize:         src.Spec.StorageConfig.StatefulSetSize,
		StoreStorageSize:        src.Spec.StorageConfig.StatefulSetSize,
		CompactStorageSize:      src.Spec.StorageConfig.StatefulSetSize,
		ReceiveStorageSize:      src.Spec.StorageConfig.StatefulSetSize,
	}

	dst.Spec.AdvancedConfig = &observabilityv1beta2.AdvancedConfig{
		RetentionConfig: &observabilityv1beta2.RetentionConfig{
			RetentionResolutionRaw: src.Spec.RetentionResolutionRaw,
			RetentionResolution5m:  src.Spec.RetentionResolution5m,
			RetentionResolution1h:  src.Spec.RetentionResolution1h,
		},
	}

	dst.Spec.EnableDownsampling = src.Spec.EnableDownSampling

	/*
		The rest of the conversion is pretty rote.
	*/
	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.ImagePullPolicy = src.Spec.ImagePullPolicy
	dst.Spec.ImagePullSecret = src.Spec.ImagePullSecret
	dst.Spec.NodeSelector = src.Spec.NodeSelector
	dst.Spec.Tolerations = src.Spec.Tolerations
	dst.Spec.ObservabilityAddonSpec = src.Spec.ObservabilityAddonSpec

	// Status
	dst.Status.Conditions = src.Status.Conditions

	// +kubebuilder:docs-gen:collapse=rote conversion
	return nil
}

/*
ConvertFrom is expected to modify its receiver to contain the converted object.
Most of the conversion is straightforward copying, except for converting our changed field.
*/

// ConvertFrom converts from the Hub version (observabilityv1beta2) to this version.
func (dst *MultiClusterObservability) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*observabilityv1beta2.MultiClusterObservability)

	// TODO(morvencao): convert the AvailabilityConfig field
	// dst.Spec.AvailabilityConfig =

	if src.Spec.AdvancedConfig != nil && src.Spec.AdvancedConfig.RetentionConfig != nil {
		dst.Spec.RetentionResolutionRaw = src.Spec.AdvancedConfig.RetentionConfig.RetentionResolutionRaw
		dst.Spec.RetentionResolution5m = src.Spec.AdvancedConfig.RetentionConfig.RetentionResolution5m
		dst.Spec.RetentionResolution1h = src.Spec.AdvancedConfig.RetentionConfig.RetentionResolution1h
	}

	dst.Spec.StorageConfig = &StorageConfigObject{
		MetricObjectStorage:     src.Spec.StorageConfig.MetricObjectStorage,
		StatefulSetStorageClass: src.Spec.StorageConfig.StorageClass,
		// How to convert the new storage size to old one?
		// StatefulSetSize =
	}

	dst.Spec.EnableDownSampling = src.Spec.EnableDownsampling

	/*
		The rest of the conversion is pretty rote.
	*/
	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.ImagePullPolicy = src.Spec.ImagePullPolicy
	dst.Spec.ImagePullSecret = src.Spec.ImagePullSecret
	dst.Spec.NodeSelector = src.Spec.NodeSelector
	dst.Spec.Tolerations = src.Spec.Tolerations
	dst.Spec.ObservabilityAddonSpec = src.Spec.ObservabilityAddonSpec

	// Status
	dst.Status.Conditions = src.Status.Conditions

	// +kubebuilder:docs-gen:collapse=rote conversion
	return nil
}
