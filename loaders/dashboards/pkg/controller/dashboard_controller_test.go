// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	hasFakeServer bool = false
)

func createDashboard() (*corev1.ConfigMap, error) {
	// read the whole file at once
	data, err := ioutil.ReadFile("../../examples/k8s-dashboard.yaml")
	if err != nil {
		panic(err)
	}

	var cm corev1.ConfigMap
	err = yaml.Unmarshal(data, &cm)
	return &cm, err
}

func createFakeServer(t *testing.T) {
	hasFakeServer = true
	server3001 := http.NewServeMux()
	server3001.HandleFunc("/api/folders",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("[{\"id\": 1,\"uid\": \"test\",\"title\": \"Custom\"}, {\"id\": 2, \"title\": \"noServer\",\"uid\": \"noServer\"}, {\"id\": 3,\"title\": \"noUID\"}]"))
		},
	)

	server3001.HandleFunc("/api/folders/id/1",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("{\"uid\": \"test\"}"))
		},
	)

	server3001.HandleFunc("/api/folders/id/2",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("{\"uid\": \"noServer\"}"))
		},
	)

	server3001.HandleFunc("/api/folders/id/3",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("{}"))
		},
	)

	server3001.HandleFunc("/api/folders/test",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("{}"))
		},
	)

	server3001.HandleFunc("/api/search",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("[]"))
		},
	)

	server3001.HandleFunc("/api/dashboards/db",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("done"))
		},
	)

	server3001.HandleFunc("/api/dashboards/uid/ff635a025bcfea7bc3dd4f508990a3e8",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("done"))
		},
	)

	err := http.ListenAndServe(":3001", server3001)
	if err != nil {
		t.Fatal("fail to create internal server at 3001")
	}
}

func TestGrafanaDashboardController(t *testing.T) {

	coreClient := fake.NewSimpleClientset().CoreV1()
	stop := make(chan struct{})

	go createFakeServer(t)
	retry = 1

	os.Setenv("POD_NAMESPACE", "ns2")

	informer := newKubeInformer(coreClient)
	go informer.Run(stop)

	cm, err := createDashboard()
	if err == nil {
		_, err := coreClient.ConfigMaps("ns2").Create(context.TODO(), cm, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("fail to create configmap with %v", err)
		}
		// wait for 2 second to trigger AddFunc of informer
		time.Sleep(time.Second * 2)
		updateDashboard(nil, cm, false)

		cm.Data = map[string]string{}
		_, err = coreClient.ConfigMaps("ns2").Update(context.TODO(), cm, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("fail to update configmap with %v", err)
		}
		// wait for 2 second to trigger UpdateFunc of informer
		time.Sleep(time.Second * 2)
		updateDashboard(nil, cm, false)

		cm, _ := createDashboard()
		_, err = coreClient.ConfigMaps("ns2").Update(context.TODO(), cm, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("fail to update configmap with %v", err)
		}

		// wait for 2 second to trigger UpdateFunc of informer
		time.Sleep(time.Second * 2)
		updateDashboard(nil, cm, false)

		coreClient.ConfigMaps("ns2").Delete(context.TODO(), cm.GetName(), metav1.DeleteOptions{})
		time.Sleep(time.Second * 2)
		deleteDashboard(cm)

	}

	close(stop)
	<-stop
}

func TestIsDesiredDashboardConfigmap(t *testing.T) {
	os.Setenv("POD_NAMESPACE", "test")
	testCaseList := []struct {
		name     string
		cm       *corev1.ConfigMap
		expected bool
	}{

		{
			"invalid cm",
			nil,
			false,
		},

		{
			"valid label",
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels:    map[string]string{"grafana-custom-dashboard": "true"},
				},
			},
			true,
		},

		{
			"valid name",
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "grafana-dashboard",
					Namespace: "test",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "MultiClusterObservability"},
					},
				},
			},
			true,
		},

		{
			"invalid label",
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels:    map[string]string{"grafana-custom-dashboard": "false"},
				},
			},
			false,
		},

		{
			"invalid name",
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "MultiClusterObservability"},
					},
				},
			},
			false,
		},

		{
			"invalid owner references",
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "test"},
					},
				},
			},
			false,
		},
	}

	for _, c := range testCaseList {
		output := isDesiredDashboardConfigmap(c.cm)
		if output != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestGetCustomFolderUID(t *testing.T) {
	if !hasFakeServer {
		go createFakeServer(t)
		retry = 1
	}

	testCaseList := []struct {
		name     string
		id       float64
		expected string
	}{

		{
			"valid folder",
			1,
			"test",
		},
		{
			"invalid folder",
			0,
			"",
		},
		{
			"no uid field",
			3,
			"",
		},
	}
	for _, c := range testCaseList {
		output := getCustomFolderUID(c.id)
		if output != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestIsEmptyFolder(t *testing.T) {
	if !hasFakeServer {
		go createFakeServer(t)
		retry = 1
	}

	testCaseList := []struct {
		name     string
		folderID float64
		expected bool
	}{

		{
			"invalid ID",
			0,
			false,
		},

		{
			"empty folder",
			1,
			true,
		},
	}

	for _, c := range testCaseList {
		output := isEmptyFolder(c.folderID)
		if output != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestGetDashboardCustomFolderTitle(t *testing.T) {
	testCaseList := []struct {
		name     string
		cm       *corev1.ConfigMap
		expected string
	}{

		{
			"invalid cm",
			nil,
			"",
		},

		{
			"default folder",
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "grafana-dashboard",
					Namespace: "test",
				},
			},
			"Custom",
		},

		{
			"general folder",
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "grafana-dashboard",
					Namespace: "test",
					Labels:    map[string]string{"general-folder": "true"},
				},
			},
			"",
		},
	}

	for _, c := range testCaseList {
		output := getDashboardCustomFolderTitle(c.cm)
		if output != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestDeleteCustomFolder(t *testing.T) {
	if !hasFakeServer {
		go createFakeServer(t)
		retry = 1
	}

	testCaseList := []struct {
		name     string
		folderID float64
		expected bool
	}{

		{
			"invalid ID",
			0,
			false,
		},

		{
			"no UID",
			3,
			false,
		},

		{
			"request error",
			2,
			false,
		},

		{
			"valid name",
			1,
			true,
		},
	}

	for _, c := range testCaseList {
		output := deleteCustomFolder(c.folderID)
		if output != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}
