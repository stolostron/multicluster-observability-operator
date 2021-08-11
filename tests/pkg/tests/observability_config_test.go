// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/utils"
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

	It("[P1][Sev1][Observability][Stable] Checking metrics default values on managed cluster (config/g0)", func() {
		if os.Getenv("SKIP_INSTALL_STEP") == "true" {
			Skip("Skip the case due to MCO CR was created customized")
		}
		mcoRes, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}
		observabilityAddonSpec := mcoRes.Object["spec"].(map[string]interface{})["observabilityAddonSpec"].(map[string]interface{})
		Expect(observabilityAddonSpec["enableMetrics"]).To(Equal(true))
		Expect(observabilityAddonSpec["interval"]).To(Equal(int64(30)))
	})

	It("[P1][Sev1][Observability][Stable] Checking default value of PVC and StorageClass (config/g0)", func() {
		if os.Getenv("SKIP_INSTALL_STEP") == "true" {
			Skip("Skip the case due to MCO CR was created customized")
		}
		mcoSC, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Get(context.TODO(), MCO_CR_NAME, metav1.GetOptions{})
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
			pvcList, err := hubClient.CoreV1().PersistentVolumeClaims(MCO_NAMESPACE).List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return err
			}
			for _, pvc := range pvcList.Items {
				//for KinD cluster, we use minio as object storage. the size is 1Gi.
				if pvc.GetName() != "minio" {
					scName := *pvc.Spec.StorageClassName
					statusPhase := pvc.Status.Phase
					if scName != expectedSC || statusPhase != "Bound" {
						return fmt.Errorf("PVC check failed, scName = %s, expectedSC = %s, statusPhase = %s", scName, expectedSC, statusPhase)
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

	It("[P1][Sev1][Observability][Integration] Checking replicas in advanced config for each component (config/g0)", func() {

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

	It("[P2][Sev2][Observability][Integration] Checking resources in advanced config (config/g0)", func() {
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
					Expect(limits["memory"]).To(Equal(deployInfo.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()))
				}
			} else {
				sts, err := utils.GetStatefulSetWithLabel(testOptions, true, component.Label, MCO_NAMESPACE)
				Expect(err).NotTo(HaveOccurred())
				for _, stsInfo := range (*sts).Items {
					Expect(cpu).To(Equal(stsInfo.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String()))
					Expect(limits["memory"]).To(Equal(stsInfo.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()))
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
