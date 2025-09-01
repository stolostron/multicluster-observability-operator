// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package health

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockManagedClusterInformer is a mock implementation of the ManagedClusterInformable interface.
type MockManagedClusterInformer struct {
	synced bool
}

func (m *MockManagedClusterInformer) Run()                                         {}
func (m *MockManagedClusterInformer) HasSynced() bool                              { return m.synced }
func (m *MockManagedClusterInformer) GetAllManagedClusterNames() map[string]struct{} { return nil }
func (m *MockManagedClusterInformer) GetManagedClusterLabelList() []string         { return nil }

func TestHealthz(t *testing.T) {
	checker := NewChecker(nil, nil, nil)
	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()

	checker.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())
}

func TestReadyz(t *testing.T) {
	// Test cases
	testCases := []struct {
		name                 string
		informerSynced       bool
		metricsServerAlive   bool
		expectedStatus       int
		expectedBodyContains string
	}{
		{
			name:                 "informer not synced",
			informerSynced:       false,
			metricsServerAlive:   true,
			expectedStatus:       http.StatusServiceUnavailable,
			expectedBodyContains: "Informer has not synced",
		},
		{
			name:                 "metrics server not alive",
			informerSynced:       true,
			metricsServerAlive:   false,
			expectedStatus:       http.StatusServiceUnavailable,
			expectedBodyContains: "Upstream metrics server check failed",
		},
		{
			name:                 "all checks pass",
			informerSynced:       true,
			metricsServerAlive:   true,
			expectedStatus:       http.StatusOK,
			expectedBodyContains: "OK",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock informer
			mockInformer := &MockManagedClusterInformer{synced: tc.informerSynced}

			// Set up mock metrics server
			var metricsServer *httptest.Server
			if tc.metricsServerAlive {
				metricsServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			} else {
				metricsServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			}
			defer metricsServer.Close()

			metricsURL, _ := url.Parse(metricsServer.URL)

			// Create checker
			checker := NewChecker(mockInformer, metricsServer.Client().Transport, metricsURL)

			// Perform request
			req := httptest.NewRequest("GET", "/readyz", nil)
			rr := httptest.NewRecorder()
			checker.ServeHTTP(rr, req)

			// Assert results
			assert.Equal(t, tc.expectedStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.expectedBodyContains)
		})
	}
}
