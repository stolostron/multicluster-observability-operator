// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"fmt"
	"k8s.io/klog/v2"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
)

func runMetricsCollectorTests(clusterConfig utils.Cluster) {
	BeforeEach(func() {
		if clusterConfig.Name == hubManagedClusterName {
			namespace = hubMetricsCollectorNamespace
			isHub = false
		}
	})

	Context("[P2][Sev2][observability] Should revert any manual changes on metrics-collector deployment (endpoint_preserve/g0) -", func() {
		newDep := &appv1.Deployment{}
		It("[Stable] Deleting metrics-collector deployment for cluster", func() {
			var (
				err error
				dep *appv1.Deployment
			)
			By(fmt.Sprintf("Handling cluster: %s", clusterConfig.Name), func() {
				Eventually(func() error {
					dep, err = utils.GetDeployment(
						clusterConfig,
						isHub,
						"metrics-collector-deployment",
						namespace,
					)
					return err
				}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(Succeed())

				Eventually(func() error {
					err = utils.DeleteDeployment(
						testOptions,
						isHub,
						"metrics-collector-deployment",
						namespace,
					)
					return err
				}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(Succeed())

				Eventually(func() bool {
					newDep, err = utils.GetDeployment(
						clusterConfig,
						isHub,
						"metrics-collector-deployment",
						namespace,
					)
					if err == nil {
						if dep.ObjectMeta.ResourceVersion != newDep.ObjectMeta.ResourceVersion {
							return true
						}
					}
					return false
				}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(BeTrue())

			})
		})
		It("[Stable] Updating metrics-collector deployment", func() {
			updateSaName := "test-serviceaccount"
			Eventually(func() error {
				newDep, err = utils.GetDeployment(
					clusterConfig,
					isHub,
					"metrics-collector-deployment",
					namespace,
				)
				if err != nil {
					return err
				}
				newDep.Spec.Template.Spec.ServiceAccountName = updateSaName
				newDep, err = utils.UpdateDeployment(
					clusterConfig,
					isHub,
					"metrics-collector-deployment",
					namespace,
					newDep,
				)
				return err
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(Succeed())

			Eventually(func() bool {
				revertDep, err := utils.GetDeployment(
					clusterConfig,
					isHub,
					"metrics-collector-deployment",
					namespace,
				)
				if err == nil {
					if revertDep.ObjectMeta.ResourceVersion != newDep.ObjectMeta.ResourceVersion &&
						revertDep.Spec.Template.Spec.ServiceAccountName != updateSaName {
						return true
					}
				}
				return false
			}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(BeTrue())
		})
	})

	It("[P2][Sev2][observability][Stable] Should revert any manual changes on metrics-collector-view clusterolebinding (endpoint_preserve/g0)", func() {
		if os.Getenv("IS_KIND_ENV") == trueStr {
			Skip("Skip the case due to run in KinD")
		}
		By("Deleting metrics-collector-view clusterolebinding")
		err, crb := utils.GetCRB(testOptions, false, "metrics-collector-view")
		Expect(err).ToNot(HaveOccurred())
		err = utils.DeleteCRB(testOptions, false, "metrics-collector-view")
		Expect(err).ToNot(HaveOccurred())
		newCrb := &rbacv1.ClusterRoleBinding{}
		Eventually(func() bool {
			err, newCrb = utils.GetCRB(testOptions, false, "metrics-collector-view")
			if err == nil {
				if crb.ObjectMeta.ResourceVersion != newCrb.ObjectMeta.ResourceVersion {
					return true
				}
			}
			return false
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(BeTrue())

		By("Updating metrics-collector-view clusterolebinding")
		updateSubName := "test-subject"
		newCrb.Subjects[0].Name = updateSubName
		err, _ = utils.UpdateCRB(testOptions, false, "metrics-collector-view", newCrb)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			err, revertCrb := utils.GetCRB(testOptions, false, "metrics-collector-view")
			if err == nil {
				if revertCrb.ObjectMeta.ResourceVersion != newCrb.ObjectMeta.ResourceVersion &&
					revertCrb.Subjects[0].Name != updateSubName {
					return true
				}
			}
			return false
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(BeTrue())
	})

	It("[P2][Sev2][observability][Stable] Should recreate on metrics-collector-serving-certs-ca-bundle configmap if deleted (endpoint_preserve/g0)", func() {
		if os.Getenv("IS_KIND_ENV") == trueStr {
			Skip("Skip the case due to run in KinD")
		}

		By("Deleting metrics-collector-serving-certs-ca-bundle configmap")
		var (
			err error
			cm  *v1.ConfigMap
		)
		Eventually(func() error {
			err, cm = utils.GetConfigMap(
				clusterConfig,
				isHub,
				"metrics-collector-serving-certs-ca-bundle",
				namespace,
			)
			return err
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(Succeed())
		Eventually(func() error {
			err = utils.DeleteConfigMap(
				clusterConfig,
				isHub,
				"metrics-collector-serving-certs-ca-bundle",
				namespace,
			)
			return err
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(Succeed())
		newCm := &v1.ConfigMap{}
		Eventually(func() bool {
			err, newCm = utils.GetConfigMap(
				clusterConfig,
				isHub,
				"metrics-collector-serving-certs-ca-bundle",
				namespace,
			)
			if err == nil {
				if cm.ObjectMeta.ResourceVersion != newCm.ObjectMeta.ResourceVersion {
					return true
				}
			}
			return false
		}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*1).Should(BeTrue())
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
		namespace = MCO_ADDON_NAMESPACE
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
		isHub = false

	})
}

var _ = Describe("Observability:", func() {
	for _, clusterConfig := range testOptions.ManagedClusters {
		klog.Error("Coleen Running metrics collector tests for cluster: ", clusterConfig.Name)
		runMetricsCollectorTests(clusterConfig)
	}
})
