// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/prometheus/alertmanager/api/v2/models"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/kustomize"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

var _ = Describe("", func() {
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

	It("RHACM4K-39481: Observability: Verify PrometheusRule resource(2.9) [P2][Sev2][Observability][Stable]@ocpInterop @post-upgrade @post-restore @e2e @post-release (alert/g1)", func() {
		By("Checking if PrometheusRule: acm-observability-alert-rules is existed")

		command := "oc"
		args := []string{"get", "prometheusrules.monitoring.coreos.com", "acm-observability-alert-rules", "-n", "open-cluster-management-observability"}

		output, err := exec.Command(command, args...).CombinedOutput()
		if err != nil {
			fmt.Printf("Error executing command: %v\n", err)
			fmt.Printf("Command output:\n%s\n", output)
			return
		}

		prometheusRule := "acm-observability-alert-rules"
		if strings.Contains(string(output), prometheusRule) {
			fmt.Println("Expected result found.")
		} else {
			fmt.Println("Expected result not found.")
		}

		fmt.Printf("Command output:\n%s\n", output)

	})

	It("RHACM4K-1404: Observability: Verify alert is created and received - Should have the expected statefulsets @BVT - [P1][Sev1][Observability][Stable]@ocpInterop @post-upgrade @post-restore @e2e @post-release (alert/g0)", func() {
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

	It("RHACM4K-1404: Observability: Verify alert is created and received - Should have the expected configmap [P2][Sev2][Observability][Stable]@ocpInterop @post-upgrade @post-restore @e2e @post-release (alert/g0)", func() {
		By("Checking if CM: thanos-ruler-default-rules is existed")
		cm, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Get(context.TODO(), configmap[0], metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(cm.ResourceVersion).ShouldNot(BeEmpty())
		klog.V(3).Infof("Configmap %s does exist", configmap[0])
	})

	It("RHACM4K-1404: Observability: Verify alert is created and received - Should not have the CM: thanos-ruler-custom-rules [P3][Sev3][Observability][Stable]@ocpInterop @post-upgrade @post-restore @e2e @post-release (alert/g0)", func() {
		By("Checking if CM: thanos-ruler-custom-rules not existed")
		_, err := hubClient.CoreV1().ConfigMaps(MCO_NAMESPACE).Get(context.TODO(), configmap[1], metav1.GetOptions{})

		if err == nil {
			err = fmt.Errorf("%s exist within the namespace env", configmap[1])
			Expect(err).NotTo(HaveOccurred())
		}

		Expect(err).To(HaveOccurred())
		klog.V(3).Infof("Configmap %s does not exist", configmap[1])
	})

	It("RHACM4K-1404: Observability: Verify alert is created and received - Should have the expected secret @BVT - [P1][Sev1][Observability][Stable]@ocpInterop @post-upgrade @post-restore @e2e @post-release (alert/g0)", func() {
		By("Checking if SECRETS: alertmanager-config is existed")
		secret, err := hubClient.CoreV1().Secrets(MCO_NAMESPACE).Get(context.TODO(), secret, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(secret.GetName()).To(Equal("alertmanager-config"))
		klog.V(3).Infof("Successfully got secret: %s", secret.GetName())
	})

	It("RHACM4K-1404: Observability: Verify alert is created and received - Should have the alertmanager configured in rule @BVT - [P1][Sev1][Observability][Stable]@ocpInterop @post-upgrade @post-restore @e2e @post-release (alert/g0)", func() {
		By("Checking if --alertmanagers.url or --alertmanager.config or --alertmanagers.config-file is configured in rule")
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

	It("RHACM4K-1404: Observability: Verify alert is created and received - Should have custom alert generated P2][Sev2][Observability][Stable]@ocpInterop @post-upgrade @post-restore @e2e @post-release (alert/g0)", func() {
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
			err, _ := utils.ContainManagedClusterMetric(testOptions, `ALERTS{`+labelName+`="`+labelValue+`"}`,
				[]string{`"__name__":"ALERTS"`, `"` + labelName + `":"` + labelValue + `"`})
			return err
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("[P2][Sev2][observability][Stable] Should modify the SECRET: alertmanager-config (alert/g0)", func() {
		By("Editing the secret, we should be able to add the third partying tools integrations")
		secret := utils.CreateCustomAlertConfigYaml(testOptions.HubCluster.BaseDomain)

		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				secret)).NotTo(HaveOccurred())
		klog.V(3).Infof("Successfully modified the secret: alertmanager-config")
	})

	It("RHACM4K-1668: Observability: Updated alert rule can take effect automatically - Should have custom alert updated [P2][Sev2][Observability][Stable]@ocpInterop @post-upgrade @post-restore @e2e @post-release (alert/g0)", func() {
		By("Updating custom alert rules")

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
				err, _ := utils.ContainManagedClusterMetric(testOptions, `ALERTS{`+labelName+`="`+labelValue+`"}`,
					[]string{`"__name__":"ALERTS"`, `"` + labelName + `":"` + labelValue + `"`})
				return err
			},
			EventuallyTimeoutMinute*5,
			EventuallyIntervalSecond*5).Should(MatchError("Failed to find metric name from response"))
	})

	It("RHACM4K-1668: Observability: Updated alert rule can take effect automatically - delete the customized rules [P2][Sev2][Observability][Stable]@ocpInterop @post-upgrade @post-restore @e2e @post-release (alert/g0)", func() {

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

	It("RHACM4K-3457: Observability: Verify managed cluster alert would be forward to hub alert manager - Should have alert named Watchdog forwarded to alertmanager [P2][Sev2][Observability][Integration]@ocpInterop @post-upgrade @post-restore @e2e (alertforward/g0)", func() {

		cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
		substring1 := "rosa"
		substring2 := "hcp"

		var amURL *url.URL

		if strings.Contains(cloudProvider, substring1) && strings.Contains(cloudProvider, substring2) {

			amURL = &url.URL{
				Scheme: "https",
				Host:   "alertmanager-open-cluster-management-observability.apps.rosa." + testOptions.HubCluster.BaseDomain,
				Path:   "/api/v2/alerts",
			}

		} else {
			amURL = &url.URL{
				Scheme: "https",
				Host:   "alertmanager-open-cluster-management-observability.apps." + testOptions.HubCluster.BaseDomain,
				Path:   "/api/v2/alerts",
			}

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

		if os.Getenv("IS_KIND_ENV") != "true" {
			if BearerToken == "" {
				BearerToken, err = utils.FetchBearerToken(testOptions)
				Expect(err).NotTo(HaveOccurred())
			}
			alertGetReq.Header.Set("Authorization", "Bearer "+BearerToken)
		}

		expectedOCPClusterIDs, err := utils.ListOCPManagedClusterIDs(testOptions, "4.8.0")
		Expect(err).NotTo(HaveOccurred())
		expectedLocalClusterIDs, err := utils.ListLocalClusterIDs(testOptions)
		expectedOCPClusterIDs = append(expectedOCPClusterIDs, expectedLocalClusterIDs...)
		klog.V(3).Infof("expectedOCPClusterIDs is %s", expectedOCPClusterIDs)
		Expect(err).NotTo(HaveOccurred())
		expectedKSClusterNames, err := utils.ListKSManagedClusterNames(testOptions)
		klog.V(3).Infof("expectedKSClusterNames is %s", expectedKSClusterNames)
		Expect(err).NotTo(HaveOccurred())
		expectClusterIdentifiers := append(expectedOCPClusterIDs, expectedKSClusterNames...)
		klog.V(3).Infof("expectClusterIdentifiers is %s", expectClusterIdentifiers)

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
				return fmt.Errorf("Failed to get alerts via alertmanager route with http reponse: %v", resp)
			}

			alertResult, err := ioutil.ReadAll(resp.Body)
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

			sort.Strings(clusterIDsInAlerts)
			klog.V(3).Infof("clusterIDsInAlerts is %s", clusterIDsInAlerts)
			sort.Strings(expectClusterIdentifiers)
			klog.V(3).Infof("sort.Strings.expectClusterIdentifiers is %s", expectClusterIdentifiers)
			klog.V(3).Infof("no sort.Strings.expectedOCPClusterIDs is %s", expectedOCPClusterIDs)
			sort.Strings(expectedOCPClusterIDs)
			klog.V(3).Infof("sort.Strings.expectedOCPClusterIDs is %s", expectedOCPClusterIDs)
			// if !reflect.DeepEqual(clusterIDsInAlerts, expectClusterIdentifiers) && !reflect.DeepEqual(clusterIDsInAlerts, expectedOCPClusterIDs) {
			if !reflect.DeepEqual(clusterIDsInAlerts, expectedOCPClusterIDs) {
				return fmt.Errorf("Not all openshift managedclusters >=4.8.0 forward Watchdog alert to hub cluster")
			}

			return nil
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(Succeed())
	})

	It("RHACM4K-22427: Observability: Disable the managedcluster's alerts forward to the Hub [P2][Sev2][Observability][Integration] @e2e (alertforward/g1)", func() {
		cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
		substring1 := "rosa"
		substring2 := "hcp"

		var amURL *url.URL

		if strings.Contains(cloudProvider, substring1) && strings.Contains(cloudProvider, substring2) {

			amURL = &url.URL{
				Scheme: "https",
				Host:   "alertmanager-open-cluster-management-observability.apps.rosa." + testOptions.HubCluster.BaseDomain,
				Path:   "/api/v2/alerts",
			}

		} else {
			amURL = &url.URL{
				Scheme: "https",
				Host:   "alertmanager-open-cluster-management-observability.apps." + testOptions.HubCluster.BaseDomain,
				Path:   "/api/v2/alerts",
			}

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

		if os.Getenv("IS_KIND_ENV") != "true" {
			if BearerToken == "" {
				BearerToken, err = utils.FetchBearerToken(testOptions)
				Expect(err).NotTo(HaveOccurred())
			}
			alertGetReq.Header.Set("Authorization", "Bearer "+BearerToken)
		}

		expectedKSClusterNames, err := utils.ListKSManagedClusterNames(testOptions)
		Expect(err).NotTo(HaveOccurred())

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

		By("Add annotations to disable alert forward")
		mco, getErr := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
		if getErr != nil {
			klog.Errorf("err: %+v\n", err)
		}
		spec := mco.Object["metadata"].(map[string]interface{})
		annotations, found := spec["annotations"].(map[string]interface{})
		if !found {
			annotations = make(map[string]interface{})
		}
		annotations["mco-disable-alerting"] = "true"
		spec["annotations"] = annotations
		_, updateErr := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Update(context.TODO(), mco, metav1.UpdateOptions{})
		if updateErr != nil {
			klog.Errorf("err: %+v\n", err)
		}

		By("Checking Watchdog alerts is not forwarded to the hub")
		Eventually(func() bool {
			resp, err := client.Do(alertGetReq)
			if err != nil {
				klog.Errorf("err: %+v\n", err)
				return false
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				klog.Errorf("err: %+v\n", resp)
				return false
			}

			alertResult, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return false
			}

			postableAlerts := models.PostableAlerts{}
			err = json.Unmarshal(alertResult, &postableAlerts)
			if err != nil {
				return false
			}
			klog.V(3).Infof("postableAlerts is %+v", postableAlerts)

			for _, alt := range postableAlerts {
				klog.V(3).Infof("alt.Labels is %s", alt.Labels)
				if alt.Labels != nil {
					klog.V(3).Infof("waiting alerts are disappeared?")
					return false
				}
			}

			return true
		}, EventuallyTimeoutMinute*10, EventuallyIntervalSecond*120).Should(BeTrue())

		klog.V(3).Infof("before enable alert forward - spec is %s", spec)
		mco1, getErr := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
		if getErr != nil {
			klog.Errorf("err: %+v\n", err)
		}
		spec1 := mco1.Object["metadata"].(map[string]interface{})
		delete(spec1["annotations"].(map[string]interface{}), "mco-disable-alerting")
		_, updateErr1 := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Update(context.TODO(), mco1, metav1.UpdateOptions{})
		if updateErr1 != nil {
			klog.Errorf("err: %+v\n", updateErr1)
		}
		klog.V(3).Infof("enable alert forward - spec1 is %s", spec1)
	})

	JustAfterEach(func() {
		Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
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
