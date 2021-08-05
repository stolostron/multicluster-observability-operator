// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-cluster-management/multicluster-observability-operator/tests/pkg/utils"
)

var (
	testStorageClassName = "test"
	testMCOCRName        = "test"
)

var _ = Describe("Observability:", func() {

	BeforeEach(func() {
		hubClient = utils.NewKubeClient(
			testOptions.HubCluster.ClusterServerURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)
	})

	It("[P2][Sev2][Observability][Integration] Create new MCO CR with a storage class that is not allow volume expansion (webhook/g0)", func() {
		By("Creating a storage class that is not allow volume expansion")
		testStorageClassForbiddenVolumeExpansionYaml := fmt.Sprintf(`
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: %s
provisioner: kubernetes.io/aws-ebs
reclaimPolicy: Retain
allowVolumeExpansion: false`, testStorageClassName)
		err := utils.Apply(testOptions.HubCluster.ClusterServerURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, []byte(testStorageClassForbiddenVolumeExpansionYaml))
		Expect(err).ToNot(HaveOccurred())

		By("Creating a new MCO CR with the storage class that is not allow volume expansion")
		testMCOCRWithStorageClassForbiddenVolumeExpansionYaml := fmt.Sprintf(`
apiVersion: observability.open-cluster-management.io/v1beta2
kind: MultiClusterObservability
metadata:
  name: %s
  annotations:
    mco-pause: "true"
spec:
  observabilityAddonSpec: {}
  storageConfig:
    storageClass: %s
    alertmanagerStorageSize: 1Gi
    compactStorageSize: 1Gi
    receiveStorageSize: 1Gi
    ruleStorageSize: 1Gi
    storeStorageSize: 1Gi
    metricObjectStorage:
      key: thanos.yaml
      name: thanos-object-storage`, testStorageClassName, testStorageClassName)
		err = utils.Apply(testOptions.HubCluster.ClusterServerURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, []byte(testMCOCRWithStorageClassForbiddenVolumeExpansionYaml))
		Expect(err).ToNot(HaveOccurred())

		By("Updating the storage size for the test MCO CR")
		testUpdatedMCOCRWithStorageClassForbiddenVolumeExpansionYaml := fmt.Sprintf(`
apiVersion: observability.open-cluster-management.io/v1beta2
kind: MultiClusterObservability
metadata:
  name: %s
spec:
  observabilityAddonSpec: {}
  storageConfig:
    storageClass: %s
    alertmanagerStorageSize: 2Gi
    compactStorageSize: 2Gi
    receiveStorageSize: 2Gi
    ruleStorageSize: 2Gi
    storeStorageSize: 2Gi
    metricObjectStorage:
      key: thanos.yaml
      name: thanos-object-storage`, testStorageClassName, testStorageClassName)
		err = utils.Apply(testOptions.HubCluster.ClusterServerURL, testOptions.KubeConfig, testOptions.HubCluster.KubeContext, []byte(testUpdatedMCOCRWithStorageClassForbiddenVolumeExpansionYaml))
		forbiddenUpdateMsg := ": Forbidden: is forbidden to update."
		Expect(err).Should(MatchError("spec.storageConfig.alertmanagerStorageSize" + forbiddenUpdateMsg))
		Expect(err).Should(MatchError("spec.storageConfig.compactStorageSize" + forbiddenUpdateMsg))
		Expect(err).Should(MatchError("spec.storageConfig.receiveStorageSize" + forbiddenUpdateMsg))
		Expect(err).Should(MatchError("spec.storageConfig.storeStorageSize" + forbiddenUpdateMsg))
		Expect(err).Should(MatchError("spec.storageConfig.ruleStorageSize" + forbiddenUpdateMsg))

		By("Deleting the testing MCO CR")
		Expect(utils.DeleteMCOInstance(testOptions, testMCOCRName)).ToNot(HaveOccurred())

		By("Deleting the testing storageclass")
		Expect(hubClient.StorageV1().StorageClasses().Delete(context.TODO(), testStorageClassName, metav1.DeleteOptions{})).ToNot(HaveOccurred())
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
