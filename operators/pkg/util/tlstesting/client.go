// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tlstesting

import (
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	tlsutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// FakeTLSClientBuilder builds a fake client for consumer and automatically
// configures a fake client for the TLS utility package setting the needed schema.
type FakeTLSClientBuilder struct {
	schemes []func(*runtime.Scheme) error
	objs    []client.Object
}

// NewFakeTLSClientBuilder creates a builder
// pre-registering the configv1 scheme needed by tlsutil.
func NewFakeTLSClientBuilder() *FakeTLSClientBuilder {
	return &FakeTLSClientBuilder{
		schemes: []func(*runtime.Scheme) error{
			configv1.AddToScheme,
		},
	}
}

func (b *FakeTLSClientBuilder) WithScheme(fn func(*runtime.Scheme) error) *FakeTLSClientBuilder {
	b.schemes = append(b.schemes, fn)
	return b
}

func (b *FakeTLSClientBuilder) WithObjects(objs ...client.Object) *FakeTLSClientBuilder {
	b.objs = append(b.objs, objs...)
	return b
}

// Build creates the fake client, injects it into the TLS utility package,
// and registers a cleanup function to reset the TLS state after the test.
func (b *FakeTLSClientBuilder) Build(t *testing.T) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	for _, addScheme := range b.schemes {
		if err := addScheme(scheme); err != nil {
			t.Fatalf("failed to add scheme: %v", err)
		}
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(b.objs...).Build()
	tlsutil.SetTLSClientFunc(func() (client.Client, error) { return c, nil })
	t.Cleanup(tlsutil.ResetTLSState)

	return c
}
