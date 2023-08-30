// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

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

	It("RHACM4K-31474: Observability: Verify memcached setting max_item_size is populated on thanos-store - [P1][Sev1][Observability][Stable]@ocpInterop @e2e(config/g1)", func() {

		By("Updating mco cr to update values in storeMemcached")
		yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: "../../../examples/maxitemsize/updatemcocr"})
		Expect(err).ToNot(HaveOccurred())
		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB,
			)).NotTo(HaveOccurred())

		time.Sleep(60 * time.Second)

		By("Check the value is effect in the sts observability-thanos-store-shard-0")
		Eventually(func() bool {

			thanosStoreMemSts, _ := utils.GetStatefulSet(testOptions, true, "observability-thanos-store-memcached", MCO_NAMESPACE)
			//klog.V(3).Infof("STS thanosStoreSts is %s", thanosStoreMemSts)
			containers := thanosStoreMemSts.Spec.Template.Spec.Containers

			args := containers[0].Args
			//klog.V(3).Infof("args is %s", args)

			argsStr := strings.Join(args, " ")
			//klog.V(3).Infof("argsStr is %s", argsStr)

			if !strings.Contains(argsStr, "-I 10m") {
				klog.V(3).Infof("maxItemSize is not effect in sts observability-thanos-store-memcached")
				return false
			}

			klog.V(3).Infof("maxItemSize is effect in sts observability-thanos-store-memcached")
			return true

		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*10).Should(BeTrue())
	})

	It("RHACM4K-31475: Observability: Verify memcached setting max_item_size is populated on thanos-query-frontend - [P1][Sev1][Observability][Stable]@ocpInterop @e2e(config/g1)", func() {

		By("Updating mco cr to update values in storeMemcached")
		yamlB, err := kustomize.Render(kustomize.Options{KustomizationPath: "../../../examples/maxitemsize/updatemcocr"})
		Expect(err).ToNot(HaveOccurred())
		Expect(
			utils.Apply(
				testOptions.HubCluster.ClusterServerURL,
				testOptions.KubeConfig,
				testOptions.HubCluster.KubeContext,
				yamlB,
			)).NotTo(HaveOccurred())

		time.Sleep(60 * time.Second)

		By("Check the value is effect in the sts observability-thanos-store-shard-0")
		Eventually(func() bool {

			thanosQueFronMemSts, _ := utils.GetStatefulSet(testOptions, true, "observability-thanos-query-frontend-memcached", MCO_NAMESPACE)
			//klog.V(3).Infof("STS thanosStoreSts is %s", thanosQueFronMemSts)
			containers := thanosQueFronMemSts.Spec.Template.Spec.Containers

			args := containers[0].Args
			//klog.V(3).Infof("args is %s", args)

			argsStr := strings.Join(args, " ")
			//klog.V(3).Infof("argsStr is %s", argsStr)

			if !strings.Contains(argsStr, "-I 10m") {
				klog.V(3).Infof("maxItemSize is not effect in sts observability-thanos-query-frontend-memcached")
				return false
			}

			klog.V(3).Infof("maxItemSize is effect in sts observability-thanos-query-frontend-memcached")
			return true

		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*10).Should(BeTrue())
	})

	It("RHACM4K-1235: Observability: Verify metrics data global setting on the managed cluster @BVT - [P1][Sev1][Observability][Stable]@ocpInterop @e2e(config/g0)", func() {
		/*
			if os.Getenv("SKIP_INSTALL_STEP") == "true" {
				Skip("Skip the case due to MCO CR was created customized")
			}
		*/
		mcoRes, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).
			Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}
		observabilityAddonSpec := mcoRes.Object["spec"].(map[string]interface{})["observabilityAddonSpec"].(map[string]interface{})
		Expect(observabilityAddonSpec["enableMetrics"]).To(Equal(true))
		Expect(observabilityAddonSpec["interval"]).To(Equal(int64(30)))
	})

	It("RHACM4K-1065: Observability: Verify MCO CR storage class and PVC @BVT - [P1][Sev1][Observability][Stable]@ocpInterop @e2e (config/g0)", func() {
		/*
			if os.Getenv("SKIP_INSTALL_STEP") == "true" {
				Skip("Skip the case due to MCO CR was created customized")
			}
		*/
		mcoSC, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).
			Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		spec := mcoSC.Object["spec"].(map[string]interface{})
		scInCR := spec["storageConfig"].(map[string]interface{})["storageClass"].(string)

		scList, _ := hubClient.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
		scMatch := false
		defaultSC := ""
		for _, sc := range scList.Items {
			if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
				defaultSC = sc.Name
			}
			if sc.Name == scInCR {
				scMatch = true
			}
		}
		expectedSC := defaultSC
		if scMatch {
			expectedSC = scInCR
		}

		Eventually(func() error {
			pvcList, err := hubClient.CoreV1().
				PersistentVolumeClaims(MCO_NAMESPACE).
				List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return err
			}
			for _, pvc := range pvcList.Items {
				//for KinD cluster, we use minio as object storage. the size is 1Gi.
				if pvc.GetName() != "minio" {
					scName := *pvc.Spec.StorageClassName
					statusPhase := pvc.Status.Phase
					if scName != expectedSC || statusPhase != "Bound" {
						return fmt.Errorf(
							"PVC check failed, scName = %s, expectedSC = %s, statusPhase = %s",
							scName,
							expectedSC,
							statusPhase,
						)
					}
				}
			}
			return nil
		}, EventuallyTimeoutMinute*3, EventuallyIntervalSecond*5).Should(Succeed())
	})

	componentMap := map[string]struct {
		// deployment or statefulset
		Type  string
		Label string
	}{
		"alertmanager": {
			Type:  "Statefulset",
			Label: ALERTMANAGER_LABEL,
		},
		"grafana": {
			Type:  "Deployment",
			Label: GRAFANA_LABEL,
		},
		"observatoriumAPI": {
			Type:  "Deployment",
			Label: OBSERVATORIUM_API_LABEL,
		},
		"rbacQueryProxy": {
			Type:  "Deployment",
			Label: RBAC_QUERY_PROXY_LABEL,
		},
		"compact": {
			Type:  "Statefulset",
			Label: THANOS_COMPACT_LABEL,
		},
		"query": {
			Type:  "Deployment",
			Label: THANOS_QUERY_LABEL,
		},
		"queryFrontend": {
			Type:  "Deployment",
			Label: THANOS_QUERY_FRONTEND_LABEL,
		},
		"queryFrontendMemcached": {
			Type:  "Statefulset",
			Label: THANOS_QUERY_FRONTEND_MEMCACHED_LABEL,
		},
		"receive": {
			Type:  "Statefulset",
			Label: THANOS_RECEIVE_LABEL,
		},
		"rule": {
			Type:  "Statefulset",
			Label: THANOS_RULE_LABEL,
		},
		"storeMemcached": {
			Type:  "Statefulset",
			Label: THANOS_STORE_MEMCACHED_LABEL,
		},
		"store": {
			Type:  "Statefulset",
			Label: THANOS_STORE_LABEL,
		},
	}

	It("RHACM4K-2822: Observability: Verify the replica in advanced config for Observability components @BVT - [P1][Sev1][Observability][Integration] @e2e (config/g0)", func() {

		mcoRes, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).
			Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}

		spec := mcoRes.Object["spec"].(map[string]interface{})
		if _, adv := spec["advanced"]; !adv {
			Skip("Skip the case since the MCO CR did not have advanced spec configed")
		}

		advancedSpec := mcoRes.Object["spec"].(map[string]interface{})["advanced"].(map[string]interface{})

		for key, component := range componentMap {
			if key == "compact" || key == "store" {
				continue
			}
			klog.V(1).Infof("The component is: %s\n", key)
			replicas := advancedSpec[key].(map[string]interface{})["replicas"]
			if component.Type == "Deployment" {
				deploys, err := utils.GetDeploymentWithLabel(testOptions, true, component.Label, MCO_NAMESPACE)
				Expect(err).NotTo(HaveOccurred())
				for _, deployInfo := range (*deploys).Items {
					Expect(int(replicas.(int64))).To(Equal(int(*deployInfo.Spec.Replicas)))
				}
			} else {
				sts, err := utils.GetStatefulSetWithLabel(testOptions, true, component.Label, MCO_NAMESPACE)
				Expect(err).NotTo(HaveOccurred())
				for _, stsInfo := range (*sts).Items {
					Expect(int(replicas.(int64))).To(Equal(int(*stsInfo.Spec.Replicas)))
				}
			}
		}
	})

	It("RHACM4K-3419: Observability: Persist advance values in MCO CR - Checking resources in advanced config [P2][Sev2][Observability][Integration] @e2e (config/g0)", func() {
		mcoRes, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}

		spec := mcoRes.Object["spec"].(map[string]interface{})
		if _, adv := spec["advanced"]; !adv {
			Skip("Skip the case since the MCO CR did not have advanced spec configed")
		}

		advancedSpec := mcoRes.Object["spec"].(map[string]interface{})["advanced"].(map[string]interface{})

		for key, component := range componentMap {
			klog.V(1).Infof("The component is: %s\n", key)
			resources := advancedSpec[key].(map[string]interface{})["resources"]
			limits := resources.(map[string]interface{})["limits"].(map[string]interface{})
			var cpu string
			switch v := limits["cpu"].(type) {
			case int64:
				cpu = fmt.Sprint(v)
			default:
				cpu = limits["cpu"].(string)
			}
			if component.Type == "Deployment" {
				deploys, err := utils.GetDeploymentWithLabel(testOptions, true, component.Label, MCO_NAMESPACE)
				Expect(err).NotTo(HaveOccurred())
				for _, deployInfo := range (*deploys).Items {
					Expect(cpu).To(Equal(deployInfo.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String()))
					Expect(
						limits["memory"],
					).To(Equal(deployInfo.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()))
				}
			} else {
				sts, err := utils.GetStatefulSetWithLabel(testOptions, true, component.Label, MCO_NAMESPACE)
				Expect(err).NotTo(HaveOccurred())
				for _, stsInfo := range (*sts).Items {
					Expect(cpu).To(Equal(stsInfo.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String()))
					memStr := stsInfo.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()
					Expect(limits["memory"]).To(Equal(memStr))
				}
			}
		}
	})

	It("RHACM4K-11169: Observability: Verify ACM Observability with Security Service Token credentials - [P2][Sev2][observability][Integration]@ocpInterop @e2e Checking service account annotations is set for store/query/rule/compact/receive (config/g0)", func() {

		mcoRes, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).
			Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}

		spec := mcoRes.Object["spec"].(map[string]interface{})
		if _, adv := spec["advanced"]; !adv {
			Skip("Skip the case since the MCO CR did not have advanced spec configed")
		}

		advancedSpec := mcoRes.Object["spec"].(map[string]interface{})["advanced"].(map[string]interface{})

		for _, component := range []string{"compact", "store", "query", "receive", "rule"} {
			klog.V(1).Infof("The component is: %s\n", component)
			annotations := advancedSpec[component].(map[string]interface{})["serviceAccountAnnotations"].(map[string]interface{})
			sas, err := utils.GetSAWithLabel(testOptions, true,
				"app.kubernetes.io/name=thanos-"+component, MCO_NAMESPACE)
			Expect(err).NotTo(HaveOccurred())
			for _, saInfo := range (*sas).Items {
				for key, value := range annotations {
					exist := false
					for eKey, eValue := range saInfo.Annotations {
						if eKey == key && eValue == value.(string) {
							exist = true
							continue
						}
					}
					Expect(exist).To(BeTrue())
				}
			}

		}
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
