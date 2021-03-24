// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1beta1

/*
For imports, we'll need the controller-runtime
[`conversion`](https://godoc.org/sigs.k8s.io/controller-runtime/pkg/conversion)
package, plus the API version for our hub type (v1beta2), and finally some of the
standard packages.
*/
import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	observabilityv1beta2 "github.com/open-cluster-management/multicluster-observability-operator/api/v1beta2"
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

	dst.Spec.StorageConfig.MetricObjectStorage = src.Spec.StorageConfig.MetricObjectStorage
	dst.Spec.StorageConfig.StorageClass = src.Spec.StorageConfig.StatefulSetStorageClass

	// How to convert the current storage size to new one?
	dst.Spec.StorageConfig.AlertmanagerStorageSize = src.Spec.StorageConfig.StatefulSetSize
	dst.Spec.StorageConfig.RuleStorageSize = src.Spec.StorageConfig.StatefulSetSize
	dst.Spec.StorageConfig.StoreStorageSize = src.Spec.StorageConfig.StatefulSetSize
	dst.Spec.StorageConfig.CompactStorageSize = src.Spec.StorageConfig.StatefulSetSize
	dst.Spec.StorageConfig.ReceiveStorageSize = src.Spec.StorageConfig.StatefulSetSize

	dst.Spec.RetentionConfig.RetentionResolutionRaw = src.Spec.RetentionResolutionRaw
	dst.Spec.RetentionConfig.RetentionResolution5m = src.Spec.RetentionResolution5m
	dst.Spec.RetentionConfig.RetentionResolution1h = src.Spec.RetentionResolution1h

	/*
		The rest of the conversion is pretty rote.
	*/
	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.EnableDownsampling = src.Spec.EnableDownSampling
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

	// TODO(morvencao): convert the StorageConfig field
	// dst.Spec.StorageConfig =

	dst.Spec.RetentionResolutionRaw = src.Spec.RetentionConfig.RetentionResolutionRaw
	dst.Spec.RetentionResolution5m = src.Spec.RetentionConfig.RetentionResolution5m
	dst.Spec.RetentionResolution1h = src.Spec.RetentionConfig.RetentionResolution1h

	dst.Spec.StorageConfig.MetricObjectStorage = src.Spec.StorageConfig.MetricObjectStorage
	dst.Spec.StorageConfig.StatefulSetStorageClass = src.Spec.StorageConfig.StorageClass
	dst.Spec.RetentionResolution1h = src.Spec.RetentionConfig.RetentionResolution1h

	// How to convert the new storage size to old one?
	// dst.Spec.StorageConfig.StatefulSetSize =

	/*
		The rest of the conversion is pretty rote.
	*/
	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.EnableDownSampling = src.Spec.EnableDownsampling
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
