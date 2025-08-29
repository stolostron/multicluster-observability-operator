// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	"testing"

	projectv1 "github.com/openshift/api/project/v1"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFetchUserProjectList(t *testing.T) {
	// Create a fake client with mock projects
	scheme := runtime.NewScheme()
	_ = projectv1.AddToScheme(scheme)
	mockProjects := &projectv1.ProjectList{
		Items: []projectv1.Project{
			{ObjectMeta: metav1.ObjectMeta{Name: "proj-a"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "proj-b"}},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithLists(mockProjects).Build()

	// Call the function with the fake client
	projectList, err := FetchUserProjectList(context.TODO(), fakeClient)

	// Assert the results
	assert.NoError(t, err)
	assert.Len(t, projectList, 2)
	assert.Contains(t, projectList, "proj-a")
	assert.Contains(t, projectList, "proj-b")
}

func TestGetUserName(t *testing.T) {
	// Create a fake client with a mock user
	scheme := runtime.NewScheme()
	_ = userv1.AddToScheme(scheme)
	mockUser := &userv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "~",
		},
	}
	// NOTE: The fake client's Get function uses the object's Name field for lookup,
	// but the real API server uses the special "~" path segment. We name our mock object
	// "~" to simulate this behavior with the fake client.
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mockUser).Build()

	// Call the function with the fake client
	userName, err := GetUserName(context.TODO(), fakeClient)

	// Assert the results
	assert.NoError(t, err)
	assert.Equal(t, "~", userName)
}
