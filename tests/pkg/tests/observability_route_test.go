// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

var (
	alertCreated bool = false
)

var _ = Describe("Observability:", func() {
	BeforeEach(func() {
		hubClient = utils.NewKubeClient(
			testOptions.HubCluster.ClusterServerURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)

		dynClient = utils.NewKubeClientDynamic(
			testOptions.HubCluster.ClusterServerURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)
	})

	It("@BVT - [P1][Sev1][Observability][Integration] Should access metrics via rbac-query-proxy route (route/g0)", func() {
		Eventually(func() error {
			query := "/api/v1/query?query=cluster_version"
			url := "https://rbac-query-proxy-open-cluster-management-observability.apps." + testOptions.HubCluster.BaseDomain + query
			req, err := http.NewRequest(
				"GET",
				url,
				nil)
			klog.V(5).Infof("request URL: %s\n", url)
			if err != nil {
				return err
			}
			caCrt, err := utils.GetRouterCA(hubClient)
			Expect(err).NotTo(HaveOccurred())
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(caCrt)
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: pool},
			}

			client := &http.Client{}
			if os.Getenv("IS_KIND_ENV") != "true" {
				client.Transport = tr
				req.Header.Set("Authorization", "Bearer "+BearerToken)
			}

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				klog.Errorf("resp: %+v\n", resp)
				klog.Errorf("err: %+v\n", err)
				return fmt.Errorf("Failed to access metrics via via rbac-query-proxy route")
			}

			metricResult, err := ioutil.ReadAll(resp.Body)
			klog.V(5).Infof("metricResult: %s\n", metricResult)
			if err != nil {
				return err
			}

			if !strings.Contains(string(metricResult), "cluster_version") {
				return fmt.Errorf("Failed to find metric name from response")
			}
			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("@BVT - [P1][Sev1][Observability][Integration] Should access alert via alertmanager route (route/g0)", func() {
		Eventually(func() error {
			query := "/api/v2/alerts"
			url := "https://alertmanager-open-cluster-management-observability.apps." + testOptions.HubCluster.BaseDomain + query
			alertJson := `
			[
				{
				   "annotations":{
					  "description":"just for mco e2e testing",
					  "summary":"an alert that is for mco e2e testing"
				   },
				   "receivers":[
					  {
						 "name":"mco-e2e"
					  }
				   ],
				   "labels":{
					  "alertname":"mco-e2e",
					  "cluster":"testCluster",
					  "severity":"none"
				   }
				}
			 ]
			`
			alertPostReq, err := http.NewRequest(
				"Post",
				url,
				bytes.NewBuffer([]byte(alertJson)))
			alertPostReq.Header.Set("Content-Type", "application/json; charset=UTF-8")
			klog.V(5).Infof("request URL: %s\n", url)
			if err != nil {
				return err
			}

			caCrt, err := utils.GetRouterCA(hubClient)
			Expect(err).NotTo(HaveOccurred())
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(caCrt)
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: pool},
			}

			client := &http.Client{}
			if os.Getenv("IS_KIND_ENV") != "true" {
				client.Transport = tr
				alertPostReq.Header.Set("Authorization", "Bearer "+BearerToken)
			}
			if !alertCreated {
				resp, err := client.Do(alertPostReq)
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					klog.Errorf("resp: %+v\n", resp)
					klog.Errorf("err: %+v\n", err)
					return fmt.Errorf("Failed to create alert via alertmanager route")
				}
			}

			alertCreated = true
			alertGetReq, err := http.NewRequest(
				"GET",
				url,
				nil)
			klog.V(5).Infof("request URL: %s\n", url)

			if err != nil {
				return err
			}

			if os.Getenv("IS_KIND_ENV") != "true" {
				alertGetReq.Header.Set("Authorization", "Bearer "+BearerToken)
			}

			resp, err := client.Do(alertGetReq)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				klog.Errorf("resp: %+v\n", resp)
				klog.Errorf("err: %+v\n", err)
				return fmt.Errorf("Failed to access alert via alertmanager route")
			}

			alertResult, err := ioutil.ReadAll(resp.Body)
			klog.V(5).Infof("alertResult: %s\n", alertResult)
			if err != nil {
				return err
			}

			if !strings.Contains(string(alertResult), "mco-e2e") {
				return fmt.Errorf("Failed to found alert from alertResult: %s", alertResult)
			}

			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			utils.PrintMCOObject(testOptions)
			utils.PrintAllMCOPodsStatus(testOptions)
			utils.PrintAllOBAPodsStatus(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
