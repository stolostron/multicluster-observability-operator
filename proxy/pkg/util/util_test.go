// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	_ "embed"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

//go:embed testdata/projects.json
var projectList string

// MockAccessReviewer is a mock implementation of the AccessReviewer interface.
type MockAccessReviewer struct {
	metricsAccess map[string][]string
	err           error
}

func (m *MockAccessReviewer) GetMetricsAccess(token string, extraArgs ...string) (map[string][]string, error) {
	return m.metricsAccess, m.err
}

func createFakeServerWithInvalidJSON(port string) *http.Server {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("invalid json"))
	})

	server := &http.Server{Addr: ":" + port, Handler: handler}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("could not listen on %s due to %s", port, err)
		}
	}()

	return server
}

func createFakeServer(port string) *http.Server {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(projectList))
	})

	server := &http.Server{Addr: ":" + port, Handler: handler}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("could not listen on %s due to %s", port, err)
		}
	}()

	return server
}

func TestFetchUserProjectList(t *testing.T) {
	// Start fake servers
	server := createFakeServer("4002")
	defer server.Close()
	invalidJSONServer := createFakeServerWithInvalidJSON("5002")
	defer invalidJSONServer.Close()
	time.Sleep(100 * time.Millisecond) // Wait a bit for the servers to start

	testCases := []struct {
		name          string
		url           string
		expectedLen   int
		expectedError bool
	}{
		{"get 2 projects from valid server", "http://127.0.0.1:4002/", 2, false},
		{"get 0 projects from invalid url", "http://127.0.0.1:300/", 0, true},
		{"get 0 projects from server with invalid json", "http://127.0.0.1:5002/", 0, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := FetchUserProjectList("", tc.url)
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Len(t, output, tc.expectedLen)
		})
	}
}

func TestWriteError(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "health")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Overwrite the package-level variable for testing.
	healthCheckFilePath = tmpFile.Name()

	writeError("test error message")
	data, err := os.ReadFile(tmpFile.Name())
	assert.NoError(t, err)
	assert.Contains(t, string(data), "test error message")
}
