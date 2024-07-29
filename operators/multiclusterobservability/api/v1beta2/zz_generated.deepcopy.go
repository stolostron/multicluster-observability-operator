//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1beta2

import (
	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AdvancedConfig) DeepCopyInto(out *AdvancedConfig) {
	*out = *in
	if in.RetentionConfig != nil {
		in, out := &in.RetentionConfig, &out.RetentionConfig
		*out = new(RetentionConfig)
		**out = **in
	}
	if in.RBACQueryProxy != nil {
		in, out := &in.RBACQueryProxy, &out.RBACQueryProxy
		*out = new(CommonSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Grafana != nil {
		in, out := &in.Grafana, &out.Grafana
		*out = new(CommonSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Alertmanager != nil {
		in, out := &in.Alertmanager, &out.Alertmanager
		*out = new(CommonSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.StoreMemcached != nil {
		in, out := &in.StoreMemcached, &out.StoreMemcached
		*out = new(CacheConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.QueryFrontendMemcached != nil {
		in, out := &in.QueryFrontendMemcached, &out.QueryFrontendMemcached
		*out = new(CacheConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.ObservatoriumAPI != nil {
		in, out := &in.ObservatoriumAPI, &out.ObservatoriumAPI
		*out = new(CommonSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.QueryFrontend != nil {
		in, out := &in.QueryFrontend, &out.QueryFrontend
		*out = new(QueryFrontendSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Query != nil {
		in, out := &in.Query, &out.Query
		*out = new(QuerySpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Compact != nil {
		in, out := &in.Compact, &out.Compact
		*out = new(CompactSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Receive != nil {
		in, out := &in.Receive, &out.Receive
		*out = new(ReceiveSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Rule != nil {
		in, out := &in.Rule, &out.Rule
		*out = new(RuleSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.Store != nil {
		in, out := &in.Store, &out.Store
		*out = new(StoreSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.MultiClusterObservabilityAddon != nil {
		in, out := &in.MultiClusterObservabilityAddon, &out.MultiClusterObservabilityAddon
		*out = new(CommonSpec)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AdvancedConfig.
func (in *AdvancedConfig) DeepCopy() *AdvancedConfig {
	if in == nil {
		return nil
	}
	out := new(AdvancedConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CacheConfig) DeepCopyInto(out *CacheConfig) {
	*out = *in
	if in.MemoryLimitMB != nil {
		in, out := &in.MemoryLimitMB, &out.MemoryLimitMB
		*out = new(int32)
		**out = **in
	}
	if in.ConnectionLimit != nil {
		in, out := &in.ConnectionLimit, &out.ConnectionLimit
		*out = new(int32)
		**out = **in
	}
	in.CommonSpec.DeepCopyInto(&out.CommonSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CacheConfig.
func (in *CacheConfig) DeepCopy() *CacheConfig {
	if in == nil {
		return nil
	}
	out := new(CacheConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CapabilitiesSpec) DeepCopyInto(out *CapabilitiesSpec) {
	*out = *in
	if in.Platform != nil {
		in, out := &in.Platform, &out.Platform
		*out = new(PlatformCapabilitiesSpec)
		**out = **in
	}
	if in.UserWorkloads != nil {
		in, out := &in.UserWorkloads, &out.UserWorkloads
		*out = new(UserWorkloadCapabilitiesSpec)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CapabilitiesSpec.
func (in *CapabilitiesSpec) DeepCopy() *CapabilitiesSpec {
	if in == nil {
		return nil
	}
	out := new(CapabilitiesSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterLogForwarderSpec) DeepCopyInto(out *ClusterLogForwarderSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterLogForwarderSpec.
func (in *ClusterLogForwarderSpec) DeepCopy() *ClusterLogForwarderSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterLogForwarderSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CommonSpec) DeepCopyInto(out *CommonSpec) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(v1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CommonSpec.
func (in *CommonSpec) DeepCopy() *CommonSpec {
	if in == nil {
		return nil
	}
	out := new(CommonSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CompactSpec) DeepCopyInto(out *CompactSpec) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(v1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.ServiceAccountAnnotations != nil {
		in, out := &in.ServiceAccountAnnotations, &out.ServiceAccountAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Containers != nil {
		in, out := &in.Containers, &out.Containers
		*out = make([]v1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CompactSpec.
func (in *CompactSpec) DeepCopy() *CompactSpec {
	if in == nil {
		return nil
	}
	out := new(CompactSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InstrumentationSpec) DeepCopyInto(out *InstrumentationSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InstrumentationSpec.
func (in *InstrumentationSpec) DeepCopy() *InstrumentationSpec {
	if in == nil {
		return nil
	}
	out := new(InstrumentationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MultiClusterObservability) DeepCopyInto(out *MultiClusterObservability) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MultiClusterObservability.
func (in *MultiClusterObservability) DeepCopy() *MultiClusterObservability {
	if in == nil {
		return nil
	}
	out := new(MultiClusterObservability)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MultiClusterObservability) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MultiClusterObservabilityList) DeepCopyInto(out *MultiClusterObservabilityList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]MultiClusterObservability, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MultiClusterObservabilityList.
func (in *MultiClusterObservabilityList) DeepCopy() *MultiClusterObservabilityList {
	if in == nil {
		return nil
	}
	out := new(MultiClusterObservabilityList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MultiClusterObservabilityList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MultiClusterObservabilitySpec) DeepCopyInto(out *MultiClusterObservabilitySpec) {
	*out = *in
	if in.Capabilities != nil {
		in, out := &in.Capabilities, &out.Capabilities
		*out = new(CapabilitiesSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.AdvancedConfig != nil {
		in, out := &in.AdvancedConfig, &out.AdvancedConfig
		*out = new(AdvancedConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]v1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.StorageConfig != nil {
		in, out := &in.StorageConfig, &out.StorageConfig
		*out = new(StorageConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.ObservabilityAddonSpec != nil {
		in, out := &in.ObservabilityAddonSpec, &out.ObservabilityAddonSpec
		*out = new(shared.ObservabilityAddonSpec)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MultiClusterObservabilitySpec.
func (in *MultiClusterObservabilitySpec) DeepCopy() *MultiClusterObservabilitySpec {
	if in == nil {
		return nil
	}
	out := new(MultiClusterObservabilitySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MultiClusterObservabilityStatus) DeepCopyInto(out *MultiClusterObservabilityStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]shared.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MultiClusterObservabilityStatus.
func (in *MultiClusterObservabilityStatus) DeepCopy() *MultiClusterObservabilityStatus {
	if in == nil {
		return nil
	}
	out := new(MultiClusterObservabilityStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenTelemetryCollectionSpec) DeepCopyInto(out *OpenTelemetryCollectionSpec) {
	*out = *in
	out.Collector = in.Collector
	out.Instrumentation = in.Instrumentation
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenTelemetryCollectionSpec.
func (in *OpenTelemetryCollectionSpec) DeepCopy() *OpenTelemetryCollectionSpec {
	if in == nil {
		return nil
	}
	out := new(OpenTelemetryCollectionSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OpenTelemetryCollectorSpec) DeepCopyInto(out *OpenTelemetryCollectorSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OpenTelemetryCollectorSpec.
func (in *OpenTelemetryCollectorSpec) DeepCopy() *OpenTelemetryCollectorSpec {
	if in == nil {
		return nil
	}
	out := new(OpenTelemetryCollectorSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PlatformCapabilitiesSpec) DeepCopyInto(out *PlatformCapabilitiesSpec) {
	*out = *in
	out.Logs = in.Logs
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PlatformCapabilitiesSpec.
func (in *PlatformCapabilitiesSpec) DeepCopy() *PlatformCapabilitiesSpec {
	if in == nil {
		return nil
	}
	out := new(PlatformCapabilitiesSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PlatformLogsCollectionSpec) DeepCopyInto(out *PlatformLogsCollectionSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PlatformLogsCollectionSpec.
func (in *PlatformLogsCollectionSpec) DeepCopy() *PlatformLogsCollectionSpec {
	if in == nil {
		return nil
	}
	out := new(PlatformLogsCollectionSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *QueryFrontendSpec) DeepCopyInto(out *QueryFrontendSpec) {
	*out = *in
	if in.Containers != nil {
		in, out := &in.Containers, &out.Containers
		*out = make([]v1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.CommonSpec.DeepCopyInto(&out.CommonSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new QueryFrontendSpec.
func (in *QueryFrontendSpec) DeepCopy() *QueryFrontendSpec {
	if in == nil {
		return nil
	}
	out := new(QueryFrontendSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *QuerySpec) DeepCopyInto(out *QuerySpec) {
	*out = *in
	if in.ServiceAccountAnnotations != nil {
		in, out := &in.ServiceAccountAnnotations, &out.ServiceAccountAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Containers != nil {
		in, out := &in.Containers, &out.Containers
		*out = make([]v1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.CommonSpec.DeepCopyInto(&out.CommonSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new QuerySpec.
func (in *QuerySpec) DeepCopy() *QuerySpec {
	if in == nil {
		return nil
	}
	out := new(QuerySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ReceiveSpec) DeepCopyInto(out *ReceiveSpec) {
	*out = *in
	if in.ServiceAccountAnnotations != nil {
		in, out := &in.ServiceAccountAnnotations, &out.ServiceAccountAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Containers != nil {
		in, out := &in.Containers, &out.Containers
		*out = make([]v1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.CommonSpec.DeepCopyInto(&out.CommonSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ReceiveSpec.
func (in *ReceiveSpec) DeepCopy() *ReceiveSpec {
	if in == nil {
		return nil
	}
	out := new(ReceiveSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RetentionConfig) DeepCopyInto(out *RetentionConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RetentionConfig.
func (in *RetentionConfig) DeepCopy() *RetentionConfig {
	if in == nil {
		return nil
	}
	out := new(RetentionConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RuleSpec) DeepCopyInto(out *RuleSpec) {
	*out = *in
	if in.ServiceAccountAnnotations != nil {
		in, out := &in.ServiceAccountAnnotations, &out.ServiceAccountAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Containers != nil {
		in, out := &in.Containers, &out.Containers
		*out = make([]v1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.CommonSpec.DeepCopyInto(&out.CommonSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RuleSpec.
func (in *RuleSpec) DeepCopy() *RuleSpec {
	if in == nil {
		return nil
	}
	out := new(RuleSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageConfig) DeepCopyInto(out *StorageConfig) {
	*out = *in
	if in.MetricObjectStorage != nil {
		in, out := &in.MetricObjectStorage, &out.MetricObjectStorage
		*out = new(shared.PreConfiguredStorage)
		**out = **in
	}
	if in.WriteStorage != nil {
		in, out := &in.WriteStorage, &out.WriteStorage
		*out = make([]*shared.PreConfiguredStorage, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(shared.PreConfiguredStorage)
				**out = **in
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageConfig.
func (in *StorageConfig) DeepCopy() *StorageConfig {
	if in == nil {
		return nil
	}
	out := new(StorageConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StoreSpec) DeepCopyInto(out *StoreSpec) {
	*out = *in
	if in.ServiceAccountAnnotations != nil {
		in, out := &in.ServiceAccountAnnotations, &out.ServiceAccountAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.CommonSpec.DeepCopyInto(&out.CommonSpec)
	if in.Containers != nil {
		in, out := &in.Containers, &out.Containers
		*out = make([]v1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StoreSpec.
func (in *StoreSpec) DeepCopy() *StoreSpec {
	if in == nil {
		return nil
	}
	out := new(StoreSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UserWorkloadCapabilitiesSpec) DeepCopyInto(out *UserWorkloadCapabilitiesSpec) {
	*out = *in
	out.Logs = in.Logs
	out.Traces = in.Traces
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UserWorkloadCapabilitiesSpec.
func (in *UserWorkloadCapabilitiesSpec) DeepCopy() *UserWorkloadCapabilitiesSpec {
	if in == nil {
		return nil
	}
	out := new(UserWorkloadCapabilitiesSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UserWorkloadLogsCollectionSpec) DeepCopyInto(out *UserWorkloadLogsCollectionSpec) {
	*out = *in
	out.ClusterLogForwarder = in.ClusterLogForwarder
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UserWorkloadLogsCollectionSpec.
func (in *UserWorkloadLogsCollectionSpec) DeepCopy() *UserWorkloadLogsCollectionSpec {
	if in == nil {
		return nil
	}
	out := new(UserWorkloadLogsCollectionSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UserWorkloadLogsSpec) DeepCopyInto(out *UserWorkloadLogsSpec) {
	*out = *in
	out.Collection = in.Collection
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UserWorkloadLogsSpec.
func (in *UserWorkloadLogsSpec) DeepCopy() *UserWorkloadLogsSpec {
	if in == nil {
		return nil
	}
	out := new(UserWorkloadLogsSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UserWorkloadTracesSpec) DeepCopyInto(out *UserWorkloadTracesSpec) {
	*out = *in
	out.Collection = in.Collection
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UserWorkloadTracesSpec.
func (in *UserWorkloadTracesSpec) DeepCopy() *UserWorkloadTracesSpec {
	if in == nil {
		return nil
	}
	out := new(UserWorkloadTracesSpec)
	in.DeepCopyInto(out)
	return out
}
