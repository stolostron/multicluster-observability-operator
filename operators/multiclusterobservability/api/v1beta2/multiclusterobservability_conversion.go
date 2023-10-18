// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package v1beta2

/*
Implementing the hub method is pretty easy -- we just have to add an empty
method called `Hub()` to serve as a
[marker](https://godoc.org/sigs.k8s.io/controller-runtime/pkg/conversion#Hub).
We could also just put this inline in our `multiclusterobservability_types.go` file.
*/

// Hub marks this type as a conversion hub.
func (*MultiClusterObservability) Hub() {}
