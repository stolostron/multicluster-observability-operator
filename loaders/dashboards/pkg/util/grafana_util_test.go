// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateUID(t *testing.T) {
	t.Parallel()
	t.Run("legacy compatibility", func(t *testing.T) {
		// Verify name-namespace order produces stable prefix
		// Using short strings to avoid hashing trigger (> 40 chars)
		name := "my-dash"
		namespace := "mco"
		uid, _ := GenerateUID(name, namespace)
		if uid != "my-dash-mco" {
			t.Fatalf("expected legacy prefix to be stable, got %v", uid)
		}
	})

	t.Run("multi-dashboard uniqueness", func(t *testing.T) {
		// Verify that different keys in the same ConfigMap produce unique UIDs
		name := "my-dashboard"
		namespace := "open-cluster-management-observability"
		key1 := "dash1.json"
		key2 := "dash2.json"

		uid1, _ := GenerateUID(name, namespace+"-"+key1)
		uid2, _ := GenerateUID(name, namespace+"-"+key2)

		if uid1 == uid2 {
			t.Fatal("UIDs for different keys should be unique")
		}
	})

	t.Run("hashing for long UIDs", func(t *testing.T) {
		uid, _ := GenerateUID("very-long-dashboard-name-that-exceeds-the-limit", "very-long-namespace-name-that-also-exceeds-the-limit")
		if len(uid) != 32 { // hex encoded fnv128 is 32 chars
			t.Fatalf("expected hashed UID to be 32 characters, got %d", len(uid))
		}
	})
}

func TestSendRequest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("done"))
		}))
		defer ts.Close()

		_, responseCode := SendRequest(t.Context(), nil, "GET", ts.URL, nil)
		if responseCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got: %v", responseCode)
		}
	})

	t.Run("payload too large", func(t *testing.T) {
		// Create a large body (maxDashboardSize + 1)
		largeBody := bytes.NewReader(make([]byte, maxDashboardSize+1))
		_, responseCode := SendRequest(t.Context(), nil, "POST", "http://127.0.0.1:1", largeBody)
		if responseCode != http.StatusRequestEntityTooLarge {
			t.Fatalf("expected 413 Payload Too Large, got: %v", responseCode)
		}
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		// Create a server and close it immediately to get a guaranteed unused dynamic port
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		unusedURL := ts.URL
		ts.Close()

		_, responseCode := SendRequest(t.Context(), nil, "GET", unusedURL, nil)
		if responseCode != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 for connection failure, got: %v", responseCode)
		}
	})

	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel() // immediately cancel

		_, responseCode := SendRequest(ctx, nil, "GET", "http://127.0.0.1:1", nil)
		if responseCode != http.StatusRequestTimeout {
			t.Fatalf("expected 408 Request Timeout, got: %v", responseCode)
		}
	})
}
