// Copyright (c) 2020 Red Hat, Inc.

package multiclusterobservability

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcov1beta1 "github.com/open-cluster-management/multicluster-monitoring-operator/pkg/apis/observability/v1beta1"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/config"
	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/util"
)

const (
	defaultReplicas    int32 = 1
	grafanaServiceName       = "grafana"
)

type GrafanaDatasources struct {
	APIVersion  int                  `yaml:"apiVersion"`
	Datasources []*GrafanaDatasource `yaml:"datasources"`
}

type GrafanaDatasource struct {
	Access            string          `yaml:"access"`
	BasicAuth         bool            `yaml:"basicAuth"`
	BasicAuthPassword string          `yaml:"basicAuthPassword"`
	BasicAuthUser     string          `yaml:"basicAuthUser"`
	Editable          bool            `yaml:"editable"`
	IsDefault         bool            `yaml:"isDefault"`
	Name              string          `yaml:"name"`
	OrgID             int             `yaml:"orgId"`
	Type              string          `yaml:"type"`
	URL               string          `yaml:"url"`
	Version           int             `yaml:"version"`
	JSONData          *JsonData       `yaml:"jsonData"`
	SecureJSONData    *SecureJsonData `yaml:"secureJsonData"`
}

type JsonData struct {
	TLSAuth   bool `yaml:"tlsAuth"`
	TLSAuthCA bool `yaml:"tlsAuthWithCACert"`
}

type SecureJsonData struct {
	TLSCACert     string `yaml:"tlsCACert"`
	TLSClientCert string `yaml:"tlsClientCert"`
	TLSClientKey  string `yaml:"tlsClientKey"`
}

// GenerateGrafanaDataSource is used to generate the GrafanaDatasource as a secret.
// the GrafanaDatasource points to observatorium api gateway service
func GenerateGrafanaDataSource(
	client client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta1.MultiClusterObservability) (*reconcile.Result, error) {

	grafanaDatasources, err := yaml.Marshal(GrafanaDatasources{
		APIVersion: 1,
		Datasources: []*GrafanaDatasource{
			{
				Name:      "Observatorium",
				Type:      "prometheus",
				Access:    "proxy",
				IsDefault: true,
				URL:       "http://rbac-query-proxy." + mco.Namespace + ".svc.cluster.local:8080",
			},
		},
	})
	if err != nil {
		return &reconcile.Result{}, err
	}

	dsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grafana-datasources",
			Namespace: mco.Namespace,
		},
		Type: "Opaque",
		StringData: map[string]string{
			"datasources.yaml": string(grafanaDatasources),
		},
	}

	// Set MultiClusterObservability instance as the owner and controller
	if err = controllerutil.SetControllerReference(mco, dsSecret, scheme); err != nil {
		return &reconcile.Result{}, err
	}

	// Check if this already exists
	grafanaDSFound := &corev1.Secret{}
	err = client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      dsSecret.Name,
			Namespace: dsSecret.Namespace,
		},
		grafanaDSFound,
	)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new grafana datasource secret",
			"dsSecret.Namespace", dsSecret.Namespace,
			"dsSecret.Name", dsSecret.Name,
		)

		err = client.Create(context.TODO(), dsSecret)
		if err != nil {
			return &reconcile.Result{}, err
		}

		// Pod created successfully - don't requeue
		return nil, nil
	} else if err != nil {
		return &reconcile.Result{}, err
	}

	return nil, nil
}

func hasCustomFolder() float64 {

	client := util.GetHTTPClient()
	grafanaURL := "http://" + grafanaServiceName + "." + config.GetDefaultNamespace() + ":3001/api/folders"
	req, _ := http.NewRequest("GET", grafanaURL, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-User", "WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000")

	resp, err := client.Do(req)
	if err != nil {
		log.Error(err, "failed to send HTTP request")
		return 0
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	folders := []map[string]interface{}{}
	err = json.Unmarshal(body, &folders)
	if err != nil {
		log.Error(err, "Failed to unmarshall data")
		return 0
	}

	for _, folder := range folders {
		if folder["title"] == "Custom" {
			return folder["id"].(float64)
		}
	}

	return 0
}

func createCustomFolder() float64 {
	client := util.GetHTTPClient()
	folderID := hasCustomFolder()
	if folderID == 0 {
		grafanaURL := "http://" + grafanaServiceName + "." + config.GetDefaultNamespace() + ":3001/api/folders"
		req, _ := http.NewRequest("POST", grafanaURL, strings.NewReader("{\"title\":\"Custom\"}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-User", "WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000")

		resp, err := client.Do(req)
		if err != nil {
			log.Error(err, "failed to send HTTP request")
			return 0
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Info("failed to parse response body", "error", err)
		} else {
			log.Info("Succeed to parse response body", "Response body", string(body))
		}

		folder := map[string]interface{}{}
		err = json.Unmarshal(body, &folder)
		return folder["id"].(float64)
	}
	return folderID
}

// UpdateDashboard is used to update the customized dashboards via calling grafana api
func UpdateDashboard(obj interface{}, overwrite bool) {

	client := util.GetHTTPClient()

	folderID := createCustomFolder()
	if folderID == 0 {
		return
	}
	for _, value := range obj.(*corev1.ConfigMap).Data {

		dashboard := map[string]interface{}{}
		err := json.Unmarshal([]byte(value), &dashboard)
		if err != nil {
			log.Error(err, "Failed to unmarshall data")
			return
		}
		dashboard["uid"] = generateUID(obj.(*corev1.ConfigMap).GetName(), obj.(*corev1.ConfigMap).GetNamespace())
		data := map[string]interface{}{
			"folderId":  folderID,
			"overwrite": overwrite,
			"dashboard": dashboard,
		}

		b, err := json.Marshal(data)
		if err != nil {
			log.Error(err, "failed to marshal body")
			return
		}

		grafanaURL := "http://" + grafanaServiceName + "." + config.GetDefaultNamespace() + ":3001/api/dashboards/db"
		req, err := http.NewRequest("POST", grafanaURL, bytes.NewBuffer(b))
		if err != nil {
			log.Error(err, "failed to new HTTP request")
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Grafana-Org-Id", "1")
		req.Header.Set("X-Forwarded-User", "WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000")

		resp, err := client.Do(req)
		if err != nil {
			log.Error(err, "failed to send HTTP request")
			return
		}

		defer resp.Body.Close()
		body1, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Info("failed to parse response body", "error", err)
		} else {
			log.Info("Succeed to parse response body", "Response body", string(body1))
		}

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusPreconditionFailed {
				if strings.Contains(string(body1), "version-mismatch") {
					UpdateDashboard(obj, true)
				} else if strings.Contains(string(body1), "name-exists") {
					log.Info("the dashboard name already existed")
				} else {
					log.Info("failed to create/update:", "", resp.StatusCode)
				}
			} else {
				log.Info("failed to create/update: ", "", resp.StatusCode)
			}
		} else {
			log.Info("Dashboard created/updated")
		}
	}

}

// DeleteDashboard ...
func DeleteDashboard(name, namespace string) {
	client := util.GetHTTPClient()

	uid := generateUID(name, namespace)
	grafanaURL := "http://" + grafanaServiceName + "." + config.GetDefaultNamespace() + ":3001/api/dashboards/uid/" + uid

	req, _ := http.NewRequest("DELETE", grafanaURL, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-User", "WHAT_YOU_ARE_DOING_IS_VOIDING_SUPPORT_0000000000000000000000000000000000000000000000000000000000000000")

	resp, err := client.Do(req)
	if err != nil {
		log.Error(err, "failed to send HTTP request")
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Info("failed to parse response body", "error", err)
	} else {
		log.Info("Succeed to parse response body", "Response body", string(body))
	}
	return
}

func generateUID(name, namespace string) string {
	uid := namespace + "-" + name
	if len(uid) > 40 {
		hasher := md5.New()
		hasher.Write([]byte(uid))
		uid = hex.EncodeToString(hasher.Sum(nil))
	}
	return uid
}
