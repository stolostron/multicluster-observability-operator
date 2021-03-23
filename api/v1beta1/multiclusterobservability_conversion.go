// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1beta1

/*
For imports, we'll need the controller-runtime
[`conversion`](https://godoc.org/sigs.k8s.io/controller-runtime/pkg/conversion)
package, plus the API version for our hub type (v1), and finally some of the
standard packages.
*/
import (
	"fmt"
	"strings"

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
// ConvertTo converts this MultiClusterObservability to the Hub version (v1).
func (src *MultiClusterObservability) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*observabilityv1beta2.MultiClusterObservability)

    // TODO(morvencao): convert the AvailabilityConfig field
	// availabilityConfig := src.Spec.AvailabilityConfig

	// TODO(morvencao): convert the StorageConfig field
	// storageConfig := src.Spec.StorageConfig

	dst.Spec.RetentionConfig.RetentionResolutionRaw = src.Spec.RetentionResolutionRaw
	dst.Spec.RetentionConfig.RetentionResolution5m = src.Spec.RetentionResolution5m
	dst.Spec.RetentionConfig.RetentionResolution1h = src.Spec.RetentionResolution1h


	/*
		The rest of the conversion is pretty rote.
	*/
	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.EnableDownsampling = src.Spec.EnableDownsampling
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

	if src.Spec.RetentionConfig.RetentionResolutionRaw != nil {
		dst.Spec.RetentionResolutionRaw = src.Spec.RetentionConfig.RetentionResolutionRaw
	}

	if src.Spec.RetentionConfig.RetentionResolution5m != nil {
		dst.Spec.RetentionResolution5m = src.Spec.RetentionConfig.RetentionResolution5m
	}

	if src.Spec.RetentionConfig.RetentionResolution1h != nil {
		dst.Spec.RetentionResolution1h = src.Spec.RetentionConfig.RetentionResolution1h
	}

	/*
		The rest of the conversion is pretty rote.
	*/
	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.EnableDownsampling = src.Spec.EnableDownsampling
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
