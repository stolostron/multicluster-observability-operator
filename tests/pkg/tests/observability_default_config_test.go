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
