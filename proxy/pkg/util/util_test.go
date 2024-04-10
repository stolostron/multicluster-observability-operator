// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package util

import (
	"context"
	stdlog "log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func newHTTPRequest() *http.Request {
	req, _ := http.NewRequest("GET", "http://127.0.0.1:3002/metrics/query?query=foo", nil)
	req.Header.Set("X-Forwarded-User", "test")
	return req
}

func createFakeServerWithInvalidJSON(port string) {
	server := http.NewServeMux()
	server.HandleFunc("/",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("invalid json"))
		},
	)
	err := http.ListenAndServe(":"+port, server)
	if err != nil {
		stdlog.Fatal("fail to create internal server at " + port)
	}
}

func createFakeServer(port string) {
	server := http.NewServeMux()
	projectList := `{
		"kind": "ProjectList",
		"apiVersion": "project.openshift.io/v1",
		"metadata": {
		  "selfLink": "/apis/project.openshift.io/v1/projects"
		},
		"items": [
		  {
			"metadata": {
			  "name": "c0",
			  "selfLink": "/apis/project.openshift.io/v1/projects/c0",
			  "uid": "2f68fd63-097c-4519-8e8f-823bb0106acc",
			  "resourceVersion": "7723",
			  "creationTimestamp": "2020-09-25T13:35:09Z",
			  "annotations": {
				"openshift.io/sa.scc.mcs": "s0:c11,c10",
				"openshift.io/sa.scc.supplemental-groups": "1000130000/10000",
				"openshift.io/sa.scc.uid-range": "1000130000/10000"
			  }
			},
			"spec": {
			  "finalizers": [
				"kubernetes"
			  ]
			},
			"status": {
			  "phase": "Active"
			}
		  },
		  {
			"metadata": {
			  "name": "c1",
			  "selfLink": "/apis/project.openshift.io/v1/projects/c1",
			  "uid": "bce1176f-6dda-45ee-99ef-675a64300643",
			  "resourceVersion": "59984227",
			  "creationTimestamp": "2020-11-26T08:34:15Z",
			  "annotations": {
				"openshift.io/sa.scc.mcs": "s0:c25,c0",
				"openshift.io/sa.scc.supplemental-groups": "1000600000/10000",
				"openshift.io/sa.scc.uid-range": "1000600000/10000"
			  }
			},
			"spec": {
			  "finalizers": [
				"kubernetes"
			  ]
			},
			"status": {
			  "phase": "Active"
			}
		  }
		]
	  }`
	server.HandleFunc("/",
		func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte(projectList))
		},
	)
	err := http.ListenAndServe(":"+port, server)
	if err != nil {
		stdlog.Fatal("fail to create internal server at " + port)
	}
}
func TestModifyMetricsQueryParams(t *testing.T) {
	testCaseList := []struct {
		name     string
		clusters map[string]string
		expected int
	}{
		{"1 cluster", map[string]string{"c0": "c0"}, 1},
		{"2 clusters", map[string]string{"c0": "c0", "c2": "c2"}, 2},
		{"no cluster", map[string]string{}, 0},
	}
	allManagedClusterNames = map[string]string{"c0": "c0", "c1": "c1"}
	for _, c := range testCaseList {
		allManagedClusterNames = c.clusters
		if len(GetAllManagedClusterNames()) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, len(GetAllManagedClusterNames()), c.expected)
		}
	}

}

func TestGetAllManagedClusterNames(t *testing.T) {
	testCaseList := []struct {
		name     string
		clusters map[string]string
		expected string
	}{
		{"do not need modify params", map[string]string{"c0": "c0"}, "query=foo"},
		{"modify params with 1 cluster", map[string]string{"c0": "c0", "c2": "c2"}, `query=foo%7Bcluster%3D%22c0%22%7D`},
		{"modify params with all cluster", map[string]string{"c0": "c0", "c1": "c1"}, `query=foo`},
		{"no cluster", map[string]string{}, "query=foo"},
	}
	go func() {
		createFakeServer("3002")
	}()
	time.Sleep(time.Second)

	InitAllManagedClusterNames()
	for _, c := range testCaseList {
		allManagedClusterNames = c.clusters
		req := newHTTPRequest()
		ModifyMetricsQueryParams(req, "http://127.0.0.1:3002/")
		if req.URL.RawQuery != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, req.URL.RawQuery, c.expected)
		}
	}
}

func TestGetAllManagedClusterLabelNames(t *testing.T) {
	testCaseList := struct {
		name             string
		managedLabelList *proxyconfig.ManagedClusterLabelList
		expected         bool
	}{"should contain enabled labels", &proxyconfig.ManagedClusterLabelList{
		IgnoreList: []string{"clusterID", "name", "environment"},
		LabelList:  []string{"cloud", "vendor"},
	}, true}

	InitAllManagedClusterLabelNames()
	updateAllManagedClusterLabelNames(testCaseList.managedLabelList)

	if isEnabled := GetAllManagedClusterLabelNames()["cloud"]; !isEnabled {
		t.Errorf("case: (%v) output: (%v) is not the expected: (%v)", testCaseList.name, isEnabled, testCaseList.expected)
	}

	if isEnabled := GetAllManagedClusterLabelNames()["vendor"]; !isEnabled {
		t.Errorf("case: (%v) output: (%v) is not the expected: (%v)", testCaseList.name, isEnabled, testCaseList.expected)
	}

	testCaseList.managedLabelList.IgnoreList = []string{"clusterID", "vendor", "environment"}
	testCaseList.managedLabelList.LabelList = []string{"cloud", "name", "environment"}
	updateAllManagedClusterLabelNames(testCaseList.managedLabelList)

	if isEnabled := GetAllManagedClusterLabelNames()["name"]; !isEnabled {
		t.Errorf("case: (%v) output: (%v) is not the expected: (%v)", testCaseList.name, isEnabled, testCaseList.expected)
	}

	if isEnabled := GetAllManagedClusterLabelNames()["vendor"]; isEnabled {
		t.Errorf("case: (%v) output: (%v) is not the expected: (%v)", testCaseList.name, isEnabled, false)
	}

	if isEnabled := GetAllManagedClusterLabelNames()["environment"]; isEnabled {
		t.Errorf("case: (%v) output: (%v) is not the expected: (%v)", testCaseList.name, isEnabled, false)
	}
}

func TestRewriteQuery(t *testing.T) {
	testCaseList := []struct {
		name        string
		urlValue    url.Values
		clusterList []string
		key         string
		expected    string
	}{
		{
			"should not rewrite",
			map[string][]string{},
			[]string{"c1", "c2"},
			"key",
			"",
		},

		{
			"should rewrite",
			map[string][]string{"key": {"value"}},
			[]string{"c1", "c2"},
			"key",
			"value{cluster=~\"c1|c2\"}",
		},

		{
			"empty cluster list",
			map[string][]string{"key": {"value"}},
			[]string{},
			"key",
			"value{cluster=~\"\"}",
		},
	}

	for _, c := range testCaseList {
		clusterMap := make(map[string][]string)
		for _, cluster := range c.clusterList {
			clusterMap[cluster] = []string{cluster}
		}
		output := rewriteQuery(c.urlValue, clusterMap, c.key)
		if output.Get(c.key) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestCanAccessAllClusters(t *testing.T) {
	testCaseList := []struct {
		name        string
		projectList []string
		clusterList map[string]string
		expected    bool
	}{
		{"no cluster and project", []string{}, map[string]string{}, false},
		{"should access all cluster", []string{"c1", "c2"}, map[string]string{"c1": "c1", "c2": "c2"}, true},
		{"should not access all cluster", []string{"c1"}, map[string]string{"c1": "c1", "c2": "c2"}, false},
		{"no project", []string{}, map[string]string{"c1": "c1", "c2": "c2"}, false},
	}

	for _, c := range testCaseList {
		allManagedClusterNames = c.clusterList
		output := canAccessAllClusters(c.projectList)
		if output != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestFetchUserProjectList(t *testing.T) {
	testCaseList := []struct {
		name     string
		token    string
		url      string
		expected int
	}{
		{"get 2 projects", "", "http://127.0.0.1:4002/", 2},
		{"invalid url", "", "http://127.0.0.1:300/", 0},
	}
	go func() {
		createFakeServer("4002")
	}()
	time.Sleep(time.Second)

	for _, c := range testCaseList {
		output := FetchUserProjectList(c.token, c.url)
		if len(output) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, len(output), c.expected)
		}
	}

	go func() {
		createFakeServerWithInvalidJSON("5002")
	}()
	output := FetchUserProjectList("", "http://127.0.0.1:5002/")
	if len(output) != 0 {
		t.Errorf("case (invalid json) output: (%v) is not the expected: (0)", len(output))
	}
}

func TestGetUserClusterList(t *testing.T) {
	testCaseList := []struct {
		name        string
		projectList []string
		clusterList map[string]string
		expected    int
	}{
		{"no project", []string{}, map[string]string{}, 0},
		{"should get 1 cluster", []string{"c1", "c2"}, map[string]string{"c1": "c1"}, 1},
		{"should get 2 cluster", []string{"c1", "c2"}, map[string]string{"c1": "c1", "c2": "c2"}, 2},
		{"no cluster", []string{"c1"}, map[string]string{}, 0},
	}

	for _, c := range testCaseList {
		allManagedClusterNames = c.clusterList
		output := getUserClusterList(c.projectList)
		if len(output) != c.expected {
			t.Errorf("case (%v) output: (%v) is not the expected: (%v)", c.name, output, c.expected)
		}
	}
}

func TestWriteError(t *testing.T) {
	writeError("test")
	data, _ := os.ReadFile("/tmp/health")
	if !strings.Contains(string(data), "test") {
		t.Errorf("failed to find the health file")
	}
}

func TestMarshalLabelListToConfigMap(t *testing.T) {
	testCase := struct {
		name     string
		obj      interface{}
		expected error
	}{
		"should marshal configmap object data correctly",
		proxyconfig.CreateManagedClusterLabelAllowListCM("ns1"),
		nil,
	}

	err := unmarshalDataToManagedClusterLabelList(testCase.obj.(*v1.ConfigMap).Data,
		proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), proxyconfig.GetManagedClusterLabelList())
	if err != nil {
		t.Errorf("failed to unmarshal configmap <%s> data to the managedLabelList: %v",
			proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), err)
	}

	err = marshalLabelListToConfigMap(testCase.obj, proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(),
		proxyconfig.GetManagedClusterLabelList())
	if err != nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, err, testCase.expected)
	}
}

func TestUnmarshalDataToManagedClusterLabelList(t *testing.T) {
	testCase := struct {
		name     string
		cm       *v1.ConfigMap
		expected error
	}{
		"should unmarshal configmap object data correctly",
		proxyconfig.CreateManagedClusterLabelAllowListCM("ns1"),
		nil,
	}

	err := unmarshalDataToManagedClusterLabelList(testCase.cm.Data,
		proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), proxyconfig.GetManagedClusterLabelList())

	if err != nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, err, testCase.expected)
	}

	testCase.cm.Data[proxyconfig.GetManagedClusterLabelAllowListConfigMapKey()] += `
	labels:
	- app
		- source
	`

	err = unmarshalDataToManagedClusterLabelList(testCase.cm.Data,
		proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), proxyconfig.GetManagedClusterLabelList())
	if err == nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, err, "unmarshal error")
	}
}

func TestGetManagedClusterEventHandler(t *testing.T) {
	testCase := struct {
		name     string
		oldObj   interface{}
		newObj   interface{}
		expected bool
	}{
		"should execute eventHandler",
		&clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: "ns1",
			},
		},
		&clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster2",
				Namespace: "ns2",
			},
		},
		false,
	}

	InitAllManagedClusterNames()
	InitAllManagedClusterLabelNames()

	eventHandler := GetManagedClusterEventHandler()
	testCase.oldObj.(*clusterv1.ManagedCluster).Labels = map[string]string{
		"name":        testCase.oldObj.(*clusterv1.ManagedCluster).Name,
		"environment": "dev",
	}

	eventHandler.AddFunc(testCase.oldObj)
	testCase.newObj.(*clusterv1.ManagedCluster).Labels = map[string]string{
		"name":        testCase.oldObj.(*clusterv1.ManagedCluster).Name,
		"environment": "dev",
		"cloud":       "Amazon",
	}
	eventHandler.UpdateFunc(testCase.oldObj, testCase.newObj)
	eventHandler.DeleteFunc(testCase.newObj)

	if ok := GetAllManagedClusterLabelNames()["cluster2"]; ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, testCase.expected)
	}
}

func TestGetManagedClusterLabelAllowListEventHandler(t *testing.T) {
	testCase := struct {
		name   string
		oldObj interface{}
		newObj interface{}
	}{
		"should execute eventHandler",
		proxyconfig.CreateManagedClusterLabelAllowListCM("open-cluster-management-observability"),
		proxyconfig.CreateManagedClusterLabelAllowListCM("open-cluster-management-observability"),
	}

	client := fake.NewSimpleClientset()
	cm, err := client.CoreV1().ConfigMaps("open-cluster-management-observability").Create(
		context.TODO(),
		proxyconfig.CreateManagedClusterLabelAllowListCM("open-cluster-management-observability"),
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Errorf("failed to create managedcluster label allowlist configmap: %v", err)
	}

	InitAllManagedClusterLabelNames()
	managedLabelList := proxyconfig.GetManagedClusterLabelList()

	err = unmarshalDataToManagedClusterLabelList(cm.Data,
		proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), managedLabelList)

	if err != nil {
		t.Errorf("failed to unmarshal managedcluster label allowlist configmap key: %s: %v",
			proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), err)
	}

	eventHandler := GetManagedClusterLabelAllowListEventHandler(client)
	InitScheduler()

	eventHandler.AddFunc(testCase.oldObj)
	if GetAllManagedClusterLabelNames() == nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, nil, nil)
	}

	time.Sleep(5 * time.Second)

	managedLabelList.IgnoreList = []string{"vendor"}
	eventHandler.UpdateFunc(testCase.oldObj, testCase.newObj)
	if ok := GetAllManagedClusterLabelNames()["vendor"]; !ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, true)
	}
	eventHandler.DeleteFunc(testCase.newObj)
}

func TestStopScheduleManagedClusterLabelAllowlistResync(t *testing.T) {
	testCase := struct {
		name     string
		expected bool
	}{
		"should stop scheduler from running",
		true,
	}

	InitScheduler()
	scheduler.Every(1).Seconds().Do(func() {})

	go scheduler.StartAsync()
	time.Sleep(6 * time.Second)

	StopScheduleManagedClusterLabelAllowlistResync()
	if ok := scheduler.IsRunning(); ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, testCase.expected)
	}
}

func TestScheduleManagedClusterLabelAllowlistResync(t *testing.T) {
	testCase := struct {
		name      string
		namespace string
		expected  int
	}{
		"should schedule a resync job for managedcluster label allowlist",
		proxyconfig.ManagedClusterLabelAllowListNamespace,
		1,
	}
	InitAllManagedClusterLabelNames()
	managedLabelList = proxyconfig.GetManagedClusterLabelList()
	managedLabelList.LabelList = []string{"cloud", "environment"}

	client := fake.NewSimpleClientset()
	client.CoreV1().ConfigMaps(testCase.namespace).Create(context.TODO(),
		proxyconfig.CreateManagedClusterLabelAllowListCM(testCase.namespace), metav1.CreateOptions{})

	go ScheduleManagedClusterLabelAllowlistResync(client)
	time.Sleep(4 * time.Second)

	updateAllManagedClusterLabelNames(managedLabelList)
	StopScheduleManagedClusterLabelAllowlistResync()
	if ok := scheduler.IsRunning(); ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, false)
	}

	go ScheduleManagedClusterLabelAllowlistResync(client)
	time.Sleep(4 * time.Second)

	StopScheduleManagedClusterLabelAllowlistResync()
	if ok := scheduler.IsRunning(); ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, testCase.expected)
	}
}

func TestResyncManagedClusterLabelAllowList(t *testing.T) {
	testCase := struct {
		name      string
		namespace string
		configmap *v1.ConfigMap
		expected  error
	}{
		"should resync managedcluster labels",
		proxyconfig.ManagedClusterLabelAllowListNamespace,
		proxyconfig.CreateManagedClusterLabelAllowListCM(proxyconfig.ManagedClusterLabelAllowListNamespace),
		nil,
	}

	InitAllManagedClusterLabelNames()
	managedLabelList = proxyconfig.GetManagedClusterLabelList()
	managedLabelList.LabelList = []string{"cloud", "environment"}

	client := fake.NewSimpleClientset()
	client.CoreV1().ConfigMaps(testCase.namespace).Create(context.TODO(), testCase.configmap, metav1.CreateOptions{})

	err := resyncManagedClusterLabelAllowList(client)
	if err != nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, err, testCase.expected)
	}
}

func TestUpdateAllManagedClusterLabelNames(t *testing.T) {
	testCase := struct {
		name     string
		expected bool
	}{
		"should update empty managedcluster label list",
		false,
	}

	managedLabelList := &proxyconfig.ManagedClusterLabelList{}
	updateAllManagedClusterLabelNames(managedLabelList)

	if ok := managedLabelList != nil; !ok {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, ok, false)
	}
}

func TestSortManagedLabelList(t *testing.T) {
	testCase := struct {
		name      string
		configmap *v1.ConfigMap
		expected  error
	}{
		"should be able to sort managed labels list",
		proxyconfig.CreateManagedClusterLabelAllowListCM("ns1"),
		nil,
	}
	InitAllManagedClusterLabelNames()
	var managedLabelList *proxyconfig.ManagedClusterLabelList

	sortManagedLabelList(managedLabelList)
	managedLabelList = proxyconfig.GetManagedClusterLabelList()

	err := unmarshalDataToManagedClusterLabelList(testCase.configmap.Data,
		proxyconfig.GetManagedClusterLabelAllowListConfigMapKey(), managedLabelList)

	if err != nil {
		t.Errorf("case (%v) output: (%v) is not the expected: (%v)", testCase.name, err, testCase.expected)
	}

	updateAllManagedClusterLabelNames(managedLabelList)
	sortManagedLabelList(managedLabelList)
}
