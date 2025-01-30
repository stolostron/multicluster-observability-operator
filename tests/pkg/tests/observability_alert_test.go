// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/prometheus/alertmanager/api/v2/models"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

const (
	trueStr = "true"
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
	statefulsetLabels := [...]string{
		ALERTMANAGER_LABEL,
		THANOS_RULE_LABEL,
	}
	configmap := [...]string{
		"thanos-ruler-default-rules",
		"thanos-ruler-custom-rules",
	}
	secret := "alertmanager-config"

	It("@BVT - [P1][Sev1][observability][Stable] Should have the expected statefulsets (alert/g0)", func() {
		By("Checking if STS: Alertmanager and observability-thanos-rule exist")
		for _, label := range statefulsetLabels {
			sts, err := hubClient.AppsV1().
				StatefulSets(MCO_NAMESPACE).
				List(context.TODO(), metav1.ListOptions{LabelSelector: label})
			Expect(err).NotTo(HaveOccurred())
			for _, stsInfo := range (*sts).Items {
				Expect(len(stsInfo.Spec.Template.Spec.Volumes)).Should(BeNumerically(">", 0))

				if strings.Contains(stsInfo.Name, "-alertmanager") {
					By("The statefulset: " + stsInfo.Name + " should have the appropriate secret mounted")
					Expect(stsInfo.Spec.Template.Spec.Volumes[0].Secret.SecretName).To(Equal("alertmanager-config"))
				}

				if strings.Contains(stsInfo.Name, "-thanos-rule") {
					By("The statefulset: " + stsInfo.Name + " should have the appropriate configmap mounted")
					Expect(
						stsInfo.Spec.Template.Spec.Volumes[0].ConfigMap.Name,
					).To(Equal("thanos-ruler-default-rules"))
				}
			}
		}
	})

	It("[P2][Sev2][observability][Stable] Should have the expected configmap (alert/g0)", func() {
		By("Checking if CM: thanos-ruler-default-rules is existed")
		cm, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Get(context.TODO(), configmap[0], metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(cm.ResourceVersion).ShouldNot(BeEmpty())
		klog.V(3).Infof("Configmap %s does exist", configmap[0])
	})

	It("[P3][Sev3][observability][Stable] Should not have the CM: thanos-ruler-custom-rules (alert/g0)", func() {
		By("Checking if CM: thanos-ruler-custom-rules not existed")
		_, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Get(context.TODO(), configmap[1], metav1.GetOptions{})

		if err == nil {
			err = fmt.Errorf("%s exist within the namespace env", configmap[1])
			Expect(err).NotTo(HaveOccurred())
		}

		Expect(err).To(HaveOccurred())
		klog.V(3).Infof("Configmap %s does not exist", configmap[1])
	})

	It("@BVT - [P1][Sev1][observability][Stable] Should have the expected secret (alert/g0)", func() {
		By("Checking if SECRETS: alertmanager-config is existed")
		secret, err := hubClient.CoreV1().Secrets(MCO_NAMESPACE).Get(context.TODO(), secret, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(secret.GetName()).To(Equal("alertmanager-config"))
		klog.V(3).Infof("Successfully got secret: %s", secret.GetName())
	})

	It("@BVT - [P1][Sev1][observability][Stable] Should have the alertmanager configured in rule (alert/g0)", func() {
		By(`Checking if --alertmanagers.url or --alertmanager.config or 
		    --alertmanagers.config-file is configured in rule`)
		rules, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
			LabelSelector: THANOS_RULE_LABEL,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(rules.Items)).NotTo(Equal(0))
		argList := (*rules).Items[0].Spec.Template.Spec.Containers[0].Args
		exists := false
		for _, arg := range argList {
			if arg == "--alertmanagers.url=http://alertmanager:9093" {
				exists = true
				break
			}
			if strings.HasPrefix(arg, "--alertmanagers.config=") {
				exists = true
				break
			}
			if strings.HasPrefix(arg, "--alertmanagers.config-file=") {
				exists = true
				break
			}
		}
		Expect(exists).To(Equal(true))
		klog.V(3).Info("Have the alertmanager url configured in rule")
	})

	It("[P2][Sev2][observability][Stable] Should have custom alert generated (alert/g0)", func() {
		By("Creating custom alert rules")

		rules, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
			LabelSelector: THANOS_RULE_LABEL,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(rules.Items)).NotTo(Equal(0))

		stsName := (*rules).Items[0].Name
		oldSts, _ := utils.GetStatefulSet(testOptions, true, stsName, MCO_NAMESPACE)

		yamlB, err := kustomize.Render(
			kustomize.Options{KustomizationPath: "../../../examples/alerts/custom_rules_valid"},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB)).NotTo(HaveOccurred())

		ThanosRuleRestarting := false
		By("Wait for thanos rule pods are restarted and ready")
		// ensure the thanos rule pods are restarted successfully before processing
		Eventually(func() error {
			if !ThanosRuleRestarting {
				newSts, _ := utils.GetStatefulSet(testOptions, true, stsName, MCO_NAMESPACE)
				if oldSts.GetResourceVersion() == newSts.GetResourceVersion() {
					return fmt.Errorf("The %s is not being restarted in 10 minutes", stsName)
				} else {
					ThanosRuleRestarting = true
				}
			}

			err = utils.CheckStatefulSetPodReady(testOptions, stsName)
			if err != nil {
				return err
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())

		var labelName, labelValue string
		labels, err := kustomize.GetLabels(yamlB)
		Expect(err).NotTo(HaveOccurred())
		for labelName = range labels.(map[string]interface{}) {
			labelValue = labels.(map[string]interface{})[labelName].(string)
		}

		By("Checking alert generated")
		Eventually(func() error {
			query := fmt.Sprintf(`ALERTS{%s="%s"}`, labelName, labelValue)
			res, err := utils.QueryGrafana(testOptions, query)
			if err != nil {
				return err
			}
			if len(res.Data.Result) == 0 {
				return fmt.Errorf("no data found for %s", query)
			}
			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})

	// TODO(subbarao): Re-enable slack integration test once issue mentioned in
	// https://github.com/stolostron/multicluster-observability-operator/pull/1112 &
	// https://github.com/stolostron/multicluster-observability-operator/pull/1258
	// is addressed.
	// It("[P2][Sev2][observability][Stable] Should modify the SECRET: alertmanager-config (alert/g0)", func() {
	// 	By("Editing the secret, we should be able to add the third partying tools integrations")
	// 	secret := utils.CreateCustomAlertConfigYaml(testOptions.HubCluster.BaseDomain)

	// 	Expect(
	// 		utils.Apply(
	// 			testOptions.HubCluster.ClusterServerURL,
	// 			testOptions.KubeConfig,
	// 			testOptions.HubCluster.KubeContext,
	// 			secret)).NotTo(HaveOccurred())
	// 	klog.V(3).Infof("Successfully modified the secret: alertmanager-config")
	// })

	It("[P2][Sev2][observability][Stable] Should have custom alert updated (alert/g0)", func() {
		By("Updating custom alert rules")

		// Replace preceding custom alert with new one that cannot fire
		yamlB, _ := kustomize.Render(
			kustomize.Options{KustomizationPath: "../../../examples/alerts/custom_rules_invalid"},
		)
		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB)).NotTo(HaveOccurred())

		var labelName, labelValue string
		labels, _ := kustomize.GetLabels(yamlB)
		for labelName = range labels.(map[string]interface{}) {
			labelValue = labels.(map[string]interface{})[labelName].(string)
		}

		By("Checking alert generated")
		Eventually(
			func() error {
				query := fmt.Sprintf(`ALERTS{%s="%s"}`, labelName, labelValue)
				res, err := utils.QueryGrafana(testOptions, query)
				if err != nil {
					return err
				}

				if len(res.Data.Result) != 0 {
					// No alert should be generated
					return fmt.Errorf("alert should not be generated, got %v", res)
				}

				return nil
			},
			EventuallyTimeoutMinute*5,
			EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][observability][Stable] delete the customized rules (alert/g0)", func() {

		rules, err := hubClient.AppsV1().StatefulSets(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{
			LabelSelector: THANOS_RULE_LABEL,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(rules.Items)).NotTo(Equal(0))

		stsName := (*rules).Items[0].Name

		oldSts, _ := utils.GetStatefulSet(testOptions, true, stsName, MCO_NAMESPACE)
		Eventually(func() error {
			err := hubClient.CoreV1().
				ConfigMaps(MCO_NAMESPACE).
				Delete(context.TODO(), configmap[1], metav1.DeleteOptions{})
			return err
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(Succeed())

		ThanosRuleRestarting := false
		By("Wait for thanos rule pods are restarted and ready")
		// ensure the thanos rule pods are restarted successfully before processing
		Eventually(func() error {
			if !ThanosRuleRestarting {
				newSts, _ := utils.GetStatefulSet(testOptions, true, stsName, MCO_NAMESPACE)
				if oldSts.GetResourceVersion() == newSts.GetResourceVersion() {
					return fmt.Errorf("The %s is not being restarted in 10 minutes", stsName)
				} else {
					ThanosRuleRestarting = true
				}
			}

			err = utils.CheckStatefulSetPodReady(testOptions, stsName)
			if err != nil {
				return err
			}
			return nil
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*5).Should(Succeed())

		klog.V(3).Infof("Successfully deleted CM: thanos-ruler-custom-rules")
	})

	It("[P2][Sev2][observability][Integration] Should have alert named Watchdog forwarded to alertmanager (alertforward/g0)", func() {
		// Watchdog is an alert that is installed by default on OCP clusters by the in-cluster monitoring stack
		// It thus exists by default on the hub and openshift spokes, and is always activated.

		amURL := url.URL{
			Scheme: "https",
			Host:   "alertmanager-open-cluster-management-observability.apps." + testOptions.HubCluster.BaseDomain,
			Path:   "/api/v2/alerts",
		}
		q := amURL.Query()
		q.Set("filter", "alertname=Watchdog")
		amURL.RawQuery = q.Encode()

		caCrt, err := utils.GetRouterCA(hubClient)
		Expect(err).NotTo(HaveOccurred())
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caCrt)

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: pool},
			},
		}

		alertGetReq, err := http.NewRequest("GET", amURL.String(), nil)
		Expect(err).NotTo(HaveOccurred())

		if os.Getenv("IS_KIND_ENV") != trueStr {
			if BearerToken == "" {
				BearerToken, err = utils.FetchBearerToken(testOptions)
				Expect(err).NotTo(HaveOccurred())
			}
			alertGetReq.Header.Set("Authorization", "Bearer "+BearerToken)
		}

		expectedOCPClusterIDs, err := utils.ListOCPManagedClusterIDs(testOptions)
		Expect(err).NotTo(HaveOccurred())
		expectedKSClusterNames, err := utils.ListKSManagedClusterNames(testOptions)
		Expect(err).NotTo(HaveOccurred())
		expectClusterIdentifiers := append(expectedOCPClusterIDs, expectedKSClusterNames...)
		missingClusters := slices.Clone(expectClusterIdentifiers)
		klog.Infof("List of cluster IDs expected to send the alert is: %s", expectClusterIdentifiers)

		// Ensure we have all the managed clusters in the list
		Expect(len(expectClusterIdentifiers)).To(Equal(len(testOptions.ManagedClusters) + 1))

		// install watchdog PrometheusRule to *KS clusters
		watchDogRuleKustomizationPath := "../../../examples/alerts/watchdog_rule"
		yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: watchDogRuleKustomizationPath})
		Expect(err).NotTo(HaveOccurred())
		for _, ks := range expectedKSClusterNames {
			for idx, mc := range testOptions.ManagedClusters {
				if mc.Name == ks {
					err = utils.Apply(
						testOptions.ManagedClusters[idx].ClusterServerURL,
						testOptions.ManagedClusters[idx].KubeConfig,
						testOptions.ManagedClusters[idx].KubeContext,
						yamlB,
					)
					Expect(err).NotTo(HaveOccurred())
				}
			}
		}

		By("Checking Watchdog alerts are forwarded to the hub")
		Eventually(func() error {
			resp, err := client.Do(alertGetReq)
			if err != nil {
				klog.Errorf("err: %+v\n", err)
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				klog.Errorf("err: %+v\n", resp)
				return fmt.Errorf("Failed to get alerts via alertmanager route with http response: %v", resp)
			}

			alertResult, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			postableAlerts := models.PostableAlerts{}
			err = json.Unmarshal(alertResult, &postableAlerts)
			if err != nil {
				return err
			}

			clusterIDsInAlerts := []string{}
			for _, alt := range postableAlerts {
				if alt.Labels != nil {
					labelSets := map[string]string(alt.Labels)
					clusterID := labelSets["managed_cluster"]
					if clusterID != "" {
						clusterIDsInAlerts = append(clusterIDsInAlerts, clusterID)
					}
				}
			}

			// Returned alerts by the alert manager is not consistent. It does not always contain all the alerts.
			// To make the test more reliable, we romove clusters found in the response continuously on each retry
			// until all have been identified in the alertmanager responses.
			for _, foundID := range clusterIDsInAlerts {
				missingClusters = slices.DeleteFunc(missingClusters, func(e string) bool { return e == foundID })
			}

			if len(missingClusters) != 0 {
				klog.Infof("Watchdog alerts are still missing from these clusters %q. Retrying...", missingClusters)
				return fmt.Errorf("Not all managedclusters forward Watchdog alert to hub cluster. Found following clusters in alerts %q. Following clusters are still missing: %q. Full list of expected clusters was: %q", clusterIDsInAlerts, missingClusters, expectClusterIdentifiers)
			}

			return nil
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			utils.LogFailingTestStandardDebugInfo(testOptions)
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
