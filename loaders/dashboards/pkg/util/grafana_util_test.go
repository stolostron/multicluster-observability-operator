// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package util

import (
	"net/http"
	"testing"
	"time"
)

func TestGenerateUID(t *testing.T) {

	uid, _ := GenerateUID("open-cluster-management", "test")
	if uid != "open-cluster-management-test" {
		t.Fatalf("the uid %v is not the expected %v", uid, "open-cluster-management-test")
	}

	uid, _ = GenerateUID("open-cluster-management-observability", "test")
	if uid != "4e20548bdba37201faabf30d1c419981" {
		t.Fatalf("the uid %v should not equal to %v", uid, "4e20548bdba37201faabf30d1c419981")
	}

}

func createFakeServer(t *testing.T) {
	server3002 := http.NewServeMux()
	server3002.HandleFunc("/",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("done"))
		},
	)
	err := http.ListenAndServe(":3002", server3002)
	if err != nil {
		t.Fatal("fail to create internal server at 3002")
	}
}

func TestSetRequest(t *testing.T) {
	go createFakeServer(t)
	time.Sleep(time.Second)
	_, responseCode := SetRequest("GET", "http://127.0.0.1:3002", nil, 1)
	if responseCode == http.StatusNotFound {
		t.Fatalf("cannot send request to server: %v", responseCode)
	}
}
