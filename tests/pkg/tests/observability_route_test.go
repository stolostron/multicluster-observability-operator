// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"errors"

	. "github.com/onsi/ginkgo/v2"
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

	It("RHACM4K-1693: Observability: Verify Observability working with new OCP API Server certs - @BVT - [P1][Sev1][observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore Should access metrics via rbac-query-proxy route @e2e (route/g0)", func() {
		Eventually(func() error {
			query := "/api/v1/query?query=cluster_version"

			cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
			substring1 := "rosa"
			substring2 := "hcp"

			var url string

			if strings.Contains(cloudProvider, substring1) && strings.Contains(cloudProvider, substring2) {
				Skip("skip on rosa-hcp")
				url = "https://rbac-query-proxy-open-cluster-management-observability.apps.rosa." + testOptions.HubCluster.BaseDomain + query

			} else {
				url = "https://rbac-query-proxy-open-cluster-management-observability.apps." + testOptions.HubCluster.BaseDomain + query

			}

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
			if os.Getenv("IS_KIND_ENV") != trueStr {
				client.Transport = tr
				client.Transport = tr
				BearerToken, err = utils.FetchBearerToken(testOptions)
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
				return errors.New("Failed to access metrics via via rbac-query-proxy route")
			}

			metricResult, err := io.ReadAll(resp.Body)
			klog.V(5).Infof("metricResult: %s\n", metricResult)
			if err != nil {
				return err
			}

			if !strings.Contains(string(metricResult), "cluster_version") {
				return errors.New("Failed to find metric name from response")
			}
			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("@BVT - [P1][Sev1][observability][Integration] Should access alert via alertmanager route (route/g0)", func() {
		Eventually(func() error {
			cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
			substring1 := "rosa"
			substring2 := "hcp"

			if strings.Contains(cloudProvider, substring1) && strings.Contains(cloudProvider, substring2) {
				Skip("skip on rosa-hcp")
			}

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
			if os.Getenv("IS_KIND_ENV") != trueStr {
				client.Transport = tr
				BearerToken, err = utils.FetchBearerToken(testOptions)
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
					return errors.New("Failed to create alert via alertmanager route")
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

			if os.Getenv("IS_KIND_ENV") != trueStr {
				BearerToken, err = utils.FetchBearerToken(testOptions)
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
				return errors.New("Failed to access alert via alertmanager route")
			}

			alertResult, err := io.ReadAll(resp.Body)
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
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
