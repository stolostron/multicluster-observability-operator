// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mcostatus "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/controllers/status"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Observability: Verify MCO status conditions [P2][Sev2][Observability][Stable] @e2e @post-release @post-upgrade @post-restore (status/g0)", func() {
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

	It("should reflect ObjectStorageSecretNotFound when the storage secret is missing", func(ctx context.Context) {
		By("Ensuring MCO is initially Ready")
		Eventually(func() error {
			return utils.CheckMCOStatusCondition(ctx, testOptions, mcostatus.ConditionTypeReady, metav1.ConditionTrue, mcostatus.ConditionTypeReady)
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())

		By("Backing up the object storage secret")
		secret, err := hubClient.CoreV1().Secrets(MCO_NAMESPACE).Get(ctx, utils.OBJ_SECRET_NAME, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		backupSecret := secret.DeepCopy()
		backupSecret.ResourceVersion = ""
		backupSecret.UID = ""

		DeferCleanup(func() {
			By("Restoring the object storage secret in Cleanup")
			_, err = hubClient.CoreV1().Secrets(MCO_NAMESPACE).Create(context.Background(), backupSecret, metav1.CreateOptions{})
			if err != nil && !k8serrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		})

		By("Deleting the object storage secret")
		err = hubClient.CoreV1().Secrets(MCO_NAMESPACE).Delete(ctx, utils.OBJ_SECRET_NAME, metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("Checking MCO status reflects the missing secret")
		Eventually(func() error {
			return utils.CheckMCOStatusCondition(ctx, testOptions, mcostatus.ConditionTypeFailed, metav1.ConditionTrue, mcostatus.ReasonObjectStorageNotFound)
		}, EventuallyTimeoutMinute*2, EventuallyIntervalSecond*5).Should(Succeed())

		By("Restoring the object storage secret")
		_, err = hubClient.CoreV1().Secrets(MCO_NAMESPACE).Create(ctx, backupSecret, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("Checking MCO status returns to Ready")
		Eventually(func() error {
			return utils.CheckMCOStatusCondition(ctx, testOptions, mcostatus.ConditionTypeReady, metav1.ConditionTrue, mcostatus.ConditionTypeReady)
		}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
	})
})
