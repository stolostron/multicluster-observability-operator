// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/open-cluster-management/multicluster-observability-operator/loaders/dashboards/pkg/util"
)

const (
	unmarshallErrMsg    = "Failed to unmarshall response body"
	customFolderKey     = "observability.open-cluster-management.io/dashboard-folder"
	generalFolderKey    = "general-folder"
	defaultCustomFolder = "Custom"
	homeDashboardTitle  = "ACM - Clusters Overview"
)

// DashboardLoader ...
type DashboardLoader struct {
	coreClient corev1client.CoreV1Interface
	informer   cache.SharedIndexInformer
}

var (
	grafanaURI = "http://127.0.0.1:3001"
	//retry on errors
	retry = 10
)

// RunGrafanaDashboardController ...
func RunGrafanaDashboardController(stop <-chan struct{}) {
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		klog.Error("Failed to get cluster config", "error", err)
	}
	// Build kubeclient client and informer for managed cluster
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatal("Failed to build kubeclient", "error", err)
	}

	go newKubeInformer(kubeClient.CoreV1()).Run(stop)
	<-stop
}

func isDesiredDashboardConfigmap(obj interface{}) bool {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok || cm == nil {
		return false
	}

	labels := cm.ObjectMeta.Labels
	if strings.ToLower(labels["grafana-custom-dashboard"]) == "true" {
		return true
	}

	owners := cm.GetOwnerReferences()
	for _, owner := range owners {
		if strings.Contains(cm.Name, "grafana-dashboard") && owner.Kind == "MultiClusterObservability" {
			return true
		}
	}

	return false
}

func newKubeInformer(coreClient corev1client.CoreV1Interface) cache.SharedIndexInformer {
	// get watched namespace
	watchedNS := os.Getenv("POD_NAMESPACE")
	watchlist := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.ConfigMaps(watchedNS).List(context.TODO(), metav1.ListOptions{})
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			return coreClient.ConfigMaps(watchedNS).Watch(context.TODO(), metav1.ListOptions{})
		},
	}
	kubeInformer := cache.NewSharedIndexInformer(
		watchlist,
		&corev1.ConfigMap{},
		time.Second*0,
		cache.Indexers{},
	)

	kubeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if !isDesiredDashboardConfigmap(obj) {
				return
			}
			klog.Infof("detect there is a new dashboard %v created", obj.(*corev1.ConfigMap).Name)
			updateDashboard(nil, obj, false)
		},
		UpdateFunc: func(old, new interface{}) {
			if old.(*corev1.ConfigMap).ObjectMeta.ResourceVersion == new.(*corev1.ConfigMap).ObjectMeta.ResourceVersion {
				return
			}
			if !isDesiredDashboardConfigmap(new) {
				return
			}
			klog.Infof("detect there is a dashboard %v updated", new.(*corev1.ConfigMap).Name)
			updateDashboard(old, new, false)
		},
		DeleteFunc: func(obj interface{}) {
			if !isDesiredDashboardConfigmap(obj) {
				return
			}
			klog.Infof("detect there is a dashboard %v deleted", obj.(*corev1.ConfigMap).Name)
			deleteDashboard(obj)
		},
	})

	return kubeInformer
}

func hasCustomFolder(folderTitle string) float64 {
	grafanaURL := grafanaURI + "/api/folders"
	body, _ := util.SetRequest("GET", grafanaURL, nil, retry)

	folders := []map[string]interface{}{}
	err := json.Unmarshal(body, &folders)
	if err != nil {
		klog.Error(unmarshallErrMsg, "error", err)
		return 0
	}

	for _, folder := range folders {
		if folder["title"] == folderTitle {
			return folder["id"].(float64)
		}
	}
	return 0
}

func createCustomFolder(folderTitle string) float64 {
	folderID := hasCustomFolder(folderTitle)
	if folderID == 0 {
		grafanaURL := grafanaURI + "/api/folders"
		body, _ := util.SetRequest("POST", grafanaURL, strings.NewReader("{\"title\":\""+folderTitle+"\"}"), retry)
		folder := map[string]interface{}{}
		err := json.Unmarshal(body, &folder)
		if err != nil {
			klog.Error(unmarshallErrMsg, "error", err)
			return 0
		}
		return folder["id"].(float64)
	}
	return folderID
}

func getCustomFolderUID(folderID float64) string {
	grafanaURL := grafanaURI + "/api/folders/id/" + fmt.Sprint(folderID)
	body, _ := util.SetRequest("GET", grafanaURL, nil, retry)
	folder := map[string]interface{}{}
	err := json.Unmarshal(body, &folder)
	if err != nil {
		klog.Error(unmarshallErrMsg, "error", err)
		return ""
	}
	uid, ok := folder["uid"]
	if ok {
		return uid.(string)
	}

	return ""
}

func isEmptyFolder(folderID float64) bool {
	if folderID == 0 {
		return false
	}

	grafanaURL := grafanaURI + "/api/search?folderIds=" + fmt.Sprint(folderID)
	body, _ := util.SetRequest("GET", grafanaURL, nil, retry)
	dashboards := []map[string]interface{}{}
	err := json.Unmarshal(body, &dashboards)
	if err != nil {
		klog.Error(unmarshallErrMsg, "error", err)
		return false
	}

	if len(dashboards) == 0 {
		klog.Infof("folder %v is empty", folderID)
		return true
	}

	return false
}

func deleteCustomFolder(folderID float64) bool {
	if folderID == 0 {
		return false
	}

	uid := getCustomFolderUID(folderID)
	if uid == "" {
		klog.Error("Failed to get custom folder UID")
		return false
	}

	grafanaURL := grafanaURI + "/api/folders/" + uid
	_, respStatusCode := util.SetRequest("DELETE", grafanaURL, nil, retry)
	if respStatusCode != http.StatusOK {
		klog.Errorf("failed to delete custom folder %v with %v", folderID, respStatusCode)
		return false
	}

	klog.Infof("custom folder %v deleted", folderID)
	return true
}

func getDashboardCustomFolderTitle(obj interface{}) string {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok || cm == nil {
		return ""
	}

	labels := cm.ObjectMeta.Labels
	if labels[generalFolderKey] == "" || strings.ToLower(labels[generalFolderKey]) != "true" {
		annotations := cm.ObjectMeta.Annotations
		customFolder, ok := annotations[customFolderKey]
		if !ok || customFolder == "" {
			customFolder = defaultCustomFolder
		}
		return customFolder
	}
	return ""
}

// updateDashboard is used to update the customized dashboards via calling grafana api
func updateDashboard(old, new interface{}, overwrite bool) {
	folderID := 0.0
	folderTitle := getDashboardCustomFolderTitle(new)
	if folderTitle != "" {
		folderID = createCustomFolder(folderTitle)
		if folderID == 0 {
			klog.Error("Failed to get custom folder id")
			return
		}
	}

	for _, value := range new.(*corev1.ConfigMap).Data {

		dashboard := map[string]interface{}{}
		err := json.Unmarshal([]byte(value), &dashboard)
		if err != nil {
			klog.Error("Failed to unmarshall data", "error", err)
			return
		}
		if dashboard["uid"] == nil {
			dashboard["uid"], _ = util.GenerateUID(new.(*corev1.ConfigMap).GetName(),
				new.(*corev1.ConfigMap).GetNamespace())
		}
		dashboard["id"] = nil
		data := map[string]interface{}{
			"folderId":  folderID,
			"overwrite": overwrite,
			"dashboard": dashboard,
		}

		b, err := json.Marshal(data)
		if err != nil {
			klog.Error("failed to marshal body", "error", err)
			return
		}

		grafanaURL := grafanaURI + "/api/dashboards/db"
		body, respStatusCode := util.SetRequest("POST", grafanaURL, bytes.NewBuffer(b), retry)

		if respStatusCode != http.StatusOK {
			if respStatusCode == http.StatusPreconditionFailed {
				if strings.Contains(string(body), "version-mismatch") {
					updateDashboard(nil, new, true)
				} else if strings.Contains(string(body), "name-exists") {
					klog.Info("the dashboard name already existed")
				} else {
					klog.Infof("failed to create/update: %v", respStatusCode)
				}
			} else {
				klog.Infof("failed to create/update: %v", respStatusCode)
			}
		} else {
			if dashboard["title"] == homeDashboardTitle {
				// get "id" value from response
				re := regexp.MustCompile("\"id\":(\\d+),")
				result := re.FindSubmatch(body)
				if len(result) != 2 {
					klog.Infof("failed to retrieve dashboard id")
				} else {
					id, err := strconv.Atoi(strings.Trim(string(result[1]), " "))
					if err != nil {
						klog.Error(err, "failed to parse dashboard id")
					} else {
						setHomeDashboard(id)
					}
				}
			}
			klog.Info("Dashboard created/updated")
		}
	}

	folderTitle = getDashboardCustomFolderTitle(old)
	folderID = hasCustomFolder(folderTitle)
	if isEmptyFolder(folderID) {
		deleteCustomFolder(folderID)
	}
}

// DeleteDashboard ...
func deleteDashboard(obj interface{}) {
	for _, value := range obj.(*corev1.ConfigMap).Data {

		dashboard := map[string]interface{}{}
		err := json.Unmarshal([]byte(value), &dashboard)
		if err != nil {
			klog.Error("Failed to unmarshall data", "error", err)
			return
		}

		uid, _ := util.GenerateUID(obj.(*corev1.ConfigMap).Name, obj.(*corev1.ConfigMap).Namespace)
		if dashboard["uid"] != nil {
			uid = dashboard["uid"].(string)
		}

		grafanaURL := grafanaURI + "/api/dashboards/uid/" + uid

		_, respStatusCode := util.SetRequest("DELETE", grafanaURL, nil, retry)
		if respStatusCode != http.StatusOK {
			klog.Errorf("failed to delete dashboard %v with %v", obj.(*corev1.ConfigMap).Name, respStatusCode)
		} else {
			klog.Info("Dashboard deleted")
		}

		folderTitle := getDashboardCustomFolderTitle(obj)
		folderID := hasCustomFolder(folderTitle)
		if isEmptyFolder(folderID) {
			deleteCustomFolder(folderID)
		}
	}
	return
}

func setHomeDashboard(id int) {
	data := map[string]int{
		"homeDashboardId": id,
	}

	b, err := json.Marshal(data)
	if err != nil {
		klog.Error("failed to marshal body", "error", err)
		return
	}
	grafanaURL := grafanaURI + "/api/org/preferences"
	_, respStatusCode := util.SetRequest("PUT", grafanaURL, bytes.NewBuffer(b), retry)

	if respStatusCode != http.StatusOK {
		klog.Infof("failed to set home dashboard: %v", respStatusCode)
	} else {
		klog.Info("Home dashboard is set")
	}
}
