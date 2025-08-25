// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package informer

import (
	"context"
	"reflect"
	"regexp"
	"slices"
	"testing"
	"time"

	proxyconfig "github.com/stolostron/multicluster-observability-operator/proxy/pkg/config"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

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
	tests := []struct {
		name              string
		labelList         []string
		ignoreList        []string
		initialLabels     map[string]bool
		expectedLabels    map[string]bool
		expectedRegexList []string
	}{
		{
			name:              "Add new labels",
			labelList:         []string{"label1", "label2"},
			ignoreList:        nil,
			initialLabels:     map[string]bool{},
			expectedLabels:    map[string]bool{"label1": true, "label2": true},
			expectedRegexList: []string{"label1", "label2"},
		},
		{
			name:              "Ignore labels",
			labelList:         nil,
			ignoreList:        []string{"label3"},
			initialLabels:     map[string]bool{"label3": true},
			expectedLabels:    map[string]bool{"label3": false},
			expectedRegexList: []string{},
		},
		{
			name:              "Add and ignore labels",
			labelList:         []string{"label4"},
			ignoreList:        []string{"label5"},
			initialLabels:     map[string]bool{"label5": true},
			expectedLabels:    map[string]bool{"label4": true, "label5": false},
			expectedRegexList: []string{"label4"},
		},
		{
			name:              "Ignore non-existing labels",
			labelList:         nil,
			ignoreList:        []string{"label6"},
			initialLabels:     map[string]bool{},
			expectedLabels:    map[string]bool{"label6": false},
			expectedRegexList: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global variable for each test
			AllManagedClusterLabelNames = tt.initialLabels
			syncLabelList = &proxyconfig.ManagedClusterLabelList{}

			managedLabelList := &proxyconfig.ManagedClusterLabelList{
				LabelList:  tt.labelList,
				IgnoreList: tt.ignoreList,
			}

			updateAllManagedClusterLabelNames(managedLabelList)

			if !reflect.DeepEqual(AllManagedClusterLabelNames, tt.expectedLabels) {
				t.Errorf("allManagedClusterLabelNames = %v, want %v", AllManagedClusterLabelNames, tt.expectedLabels)
			}

			regex := regexp.MustCompile(`[^\w]+`)
			expectedRegexList := []string{}
			for key, isEnabled := range tt.expectedLabels {
				if isEnabled {
					expectedRegexList = append(expectedRegexList, regex.ReplaceAllString(key, "_"))
				}
			}

			// The label list does not appear to be deterministically sorted
			// Sorting here in order to ensure the test can pass reliably.
			slices.Sort(syncLabelList.RegexLabelList)
			slices.Sort(expectedRegexList)
			if !reflect.DeepEqual(syncLabelList.RegexLabelList, expectedRegexList) {
				t.Errorf("syncLabelList.RegexLabelList = %v, want %v", syncLabelList.RegexLabelList, expectedRegexList)
			}
		})
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
