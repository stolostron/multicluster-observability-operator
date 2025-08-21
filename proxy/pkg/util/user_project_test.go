// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetUserProjectList(t *testing.T) {
	testCases := []struct {
		name        string
		setup       func(upi *UserProjectInfo)
		tokenToGet  string
		expectFound bool
	}{
		{
			name: "should find existing user project",
			setup: func(upi *UserProjectInfo) {
				upi.UpdateUserProject("user1", "token1", []string{"p1"})
			},
			tokenToGet:  "token1",
			expectFound: true,
		},
		{
			name: "should not find non-existing user project",
			setup: func(upi *UserProjectInfo) {
				upi.UpdateUserProject("user1", "token1", []string{"p1"})
			},
			tokenToGet:  "invalid-token",
			expectFound: false,
		},
		{
			name:        "should not find project in empty cache",
			setup:       func(upi *UserProjectInfo) {},
			tokenToGet:  "any-token",
			expectFound: false,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			upi := NewUserProjectInfo(time.Hour, defaultCleanPeriod)
			t.Cleanup(upi.Stop)

			c.setup(upi)
			_, found := upi.GetUserProjectList(c.tokenToGet)
			assert.Equal(t, c.expectFound, found)
		})
	}
}

func TestCleanExpiredProjectInfo(t *testing.T) {
	expiredDuration := 10 * time.Millisecond
	upi := NewUserProjectInfo(expiredDuration, defaultCleanPeriod)
	t.Cleanup(upi.Stop)

	// Add three users, one of whom will be updated to not expire.
	upi.UpdateUserProject("user-expired-1", "token-expired-1", []string{"p1"})
	upi.UpdateUserProject("user-valid", "token-valid", []string{"p2"})
	upi.UpdateUserProject("user-expired-2", "token-expired-2", []string{"p3"})

	// Wait for the expiration period to pass.
	time.Sleep(expiredDuration * 2)

	// Update one user to reset their timestamp.
	upi.UpdateUserProject("user-valid", "token-valid", []string{"p2-updated"})

	// Manually trigger the cleanup.
	upi.cleanExpiredProjectInfo()

	// Check that expired users are gone.
	_, found := upi.GetUserProjectList("token-expired-1")
	assert.False(t, found, "user-expired-1 should have been cleaned up")
	_, found = upi.GetUserProjectList("token-expired-2")
	assert.False(t, found, "user-expired-2 should have been cleaned up")

	// Check that the valid user remains.
	projects, found := upi.GetUserProjectList("token-valid")
	assert.True(t, found, "user-valid should not have been cleaned up")
	assert.Equal(t, []string{"p2-updated"}, projects)
}

func TestAutoCleanAndStop(t *testing.T) {
	expiredDuration := 50 * time.Millisecond
	cleanPeriod := 20 * time.Millisecond

	upi := NewUserProjectInfo(expiredDuration, cleanPeriod)
	t.Cleanup(upi.Stop)

	// 1. Test that auto-cleaning works.
	upi.UpdateUserProject("user-to-expire", "token-to-expire", []string{"p1"})
	_, found := upi.GetUserProjectList("token-to-expire")
	assert.True(t, found)

	// Wait long enough for the auto-cleaner to run at least once.
	time.Sleep(expiredDuration + cleanPeriod)

	_, found = upi.GetUserProjectList("token-to-expire")
	assert.False(t, found, "auto-cleaner should have removed the expired user")

	// 2. Test that Stop() prevents further cleaning.
	upi.UpdateUserProject("user-after-stop", "token-after-stop", []string{"p2"})
	_, found = upi.GetUserProjectList("token-after-stop")
	assert.True(t, found)

	// Stop the cleaner.
	upi.Stop()

	// Wait long enough that the user would have expired and been cleaned.
	time.Sleep(expiredDuration + cleanPeriod)

	// Check that the user is still there because the cleaner was stopped.
	_, found = upi.GetUserProjectList("token-after-stop")
	assert.True(t, found, "user should not be cleaned up after Stop() is called")
}
