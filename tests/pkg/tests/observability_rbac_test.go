// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-observability-operator/tests/pkg/utils"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	ocpUserGVR = schema.GroupVersionResource{
		Group:    "user.openshift.io",
		Version:  "v1",
		Resource: "users",
	}
	ocpOAuthAccessTokenGVR = schema.GroupVersionResource{
		Group:    "oauth.openshift.io",
		Version:  "v1",
		Resource: "oauthaccesstokens",
	}
	rbacTestUsers = []string{"admin", "user1", "user2"}
)

func rbacRawTokenForUser(username string) string {
	return fmt.Sprintf("mco-e2e-rbac-test-token-%s-0123456789abcdef", username)
}

func rbacTokenObjectName(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return "sha256~" + base64.RawURLEncoding.EncodeToString(sum[:])
}

func rbacBearerTokenForUser(username string) string {
	return "sha256~" + rbacRawTokenForUser(username)
}

func cleanupRBACTestResources(ctx context.Context) {
	hubClient := utils.NewKubeClient(
		testOptions.HubCluster.ClusterServerURL,
		testOptions.KubeConfig,
		testOptions.HubCluster.KubeContext)
	dynClient := utils.NewKubeClientDynamic(
		testOptions.HubCluster.ClusterServerURL,
		testOptions.KubeConfig,
		testOptions.HubCluster.KubeContext)

	_ = hubClient.RbacV1().ClusterRoleBindings().Delete(ctx, "cluster-manager-admin-binding", metav1.DeleteOptions{})
	_ = hubClient.RbacV1().RoleBindings("local-cluster").Delete(ctx, "view-binding-user1", metav1.DeleteOptions{})

	for _, username := range rbacTestUsers {
		tokenName := rbacTokenObjectName(rbacRawTokenForUser(username))
		_ = dynClient.Resource(ocpOAuthAccessTokenGVR).Delete(ctx, tokenName, metav1.DeleteOptions{})
		_ = dynClient.Resource(ocpUserGVR).Delete(ctx, username, metav1.DeleteOptions{})
	}
}

func setupRBACTestResources(ctx context.Context) error {
	cleanupRBACTestResources(ctx)

	hubClient := utils.NewKubeClient(
		testOptions.HubCluster.ClusterServerURL,
		testOptions.KubeConfig,
		testOptions.HubCluster.KubeContext)
	dynClient := utils.NewKubeClientDynamic(
		testOptions.HubCluster.ClusterServerURL,
		testOptions.KubeConfig,
		testOptions.HubCluster.KubeContext)

	for _, username := range rbacTestUsers {
		userObj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "user.openshift.io/v1",
				"kind":       "User",
				"metadata": map[string]any{
					"name": username,
				},
			},
		}
		createdUser, err := dynClient.Resource(ocpUserGVR).Create(ctx, userObj, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create OpenShift User %s: %w", username, err)
		}
		if apierrors.IsAlreadyExists(err) {
			createdUser, err = dynClient.Resource(ocpUserGVR).Get(ctx, username, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get existing OpenShift User %s: %w", username, err)
			}
		}

		rawToken := rbacRawTokenForUser(username)
		tokenName := rbacTokenObjectName(rawToken)
		tokenObj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "oauth.openshift.io/v1",
				"kind":       "OAuthAccessToken",
				"metadata": map[string]any{
					"name": tokenName,
				},
				"clientName":  "openshift-challenging-client",
				"expiresIn":   int64(86400),
				"scopes":      []any{"user:full"},
				"userName":    username,
				"userUID":     string(createdUser.GetUID()),
				"redirectURI": "https://127.0.0.1:12000/oauth/token/implicit",
			},
		}
		if _, err := dynClient.Resource(ocpOAuthAccessTokenGVR).Create(ctx, tokenObj, metav1.CreateOptions{}); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create OAuthAccessToken for user %s: %w", username, err)
			}
			if err := dynClient.Resource(ocpOAuthAccessTokenGVR).Delete(ctx, tokenName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete stale OAuthAccessToken for user %s: %w", username, err)
			}
			if _, err := dynClient.Resource(ocpOAuthAccessTokenGVR).Create(ctx, tokenObj, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("failed to recreate OAuthAccessToken for user %s: %w", username, err)
			}
		}
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-manager-admin-binding",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "open-cluster-management:cluster-manager-admin",
		},
		Subjects: []rbacv1.Subject{
			{
				APIGroup: rbacv1.GroupName,
				Kind:     rbacv1.UserKind,
				Name:     "admin",
			},
		},
	}
	if _, err := hubClient.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create cluster-manager-admin-binding: %w", err)
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "view-binding-user1",
			Namespace: "local-cluster",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "view",
		},
		Subjects: []rbacv1.Subject{
			{
				APIGroup: rbacv1.GroupName,
				Kind:     rbacv1.UserKind,
				Name:     "user1",
			},
		},
	}
	if _, err := hubClient.RbacV1().RoleBindings("local-cluster").Create(ctx, rb, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create view-binding-user1: %w", err)
	}

	return nil
}

var _ = Describe("", Ordered, func() {
	BeforeAll(func(ctx SpecContext) {
		Expect(setupRBACTestResources(ctx)).To(Succeed())
		DeferCleanup(func(cleanupCtx SpecContext) {
			os.Unsetenv("USER_TOKEN")
			cleanupRBACTestResources(cleanupCtx)
		})
	})

	It(
		"RHACM4K-1406 - Observability - RBAC - only authorized user could query managed cluster metrics data [Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (requires-ocp/g0) (obs_rbac/g0)",
		func() {
			By("Querying managed cluster metrics data as admin", func() {
				Expect(os.Setenv("USER_TOKEN", rbacBearerTokenForUser("admin"))).To(Succeed())
				Eventually(func() error {
					res, err := utils.QueryGrafana(testOptions, "node_memory_MemAvailable_bytes")
					if err != nil {
						return err
					}
					if len(res.Data.Result) == 0 {
						return fmt.Errorf("no data found for node_memory_MemAvailable_bytes")
					}
					return nil
				}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
			})
			By("Querying metrics data as user1 with view role in local-cluster ns", func() {
				Expect(os.Setenv("USER_TOKEN", rbacBearerTokenForUser("user1"))).To(Succeed())
				Eventually(func() error {
					res, err := utils.QueryGrafana(testOptions, "node_memory_MemAvailable_bytes{cluster=\"local-cluster\"}")
					if err != nil {
						return err
					}
					if len(res.Data.Result) == 0 {
						return fmt.Errorf("no data found for node_memory_MemAvailable_bytes{cluster=\"local-cluster\"}")
					}
					return nil
				}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
			})
			By("Querying managed cluster metrics data as user2 with no role binding", func() {
				Expect(os.Setenv("USER_TOKEN", rbacBearerTokenForUser("user2"))).To(Succeed())
				Eventually(func() error {
					res, err := utils.QueryGrafana(testOptions, "node_memory_MemAvailable_bytes{cluster=\"local-cluster\"}")
					if err != nil {
						return err
					}
					if len(res.Data.Result) != 0 {
						return fmt.Errorf("data found for node_memory_MemAvailable_bytes{cluster=\"local-cluster\"}")
					}
					return nil
				}, EventuallyTimeoutMinute*5, EventuallyIntervalSecond*5).Should(Succeed())
			})
		},
	)

	It(
		"RHACM4K-1439 - Observability - RBAC - Verify only cluster-manager-admin role can deploy MCO CR [Observability][Integration]@ocpInterop @non-ui-post-restore @non-ui-post-release @non-ui-pre-upgrade @non-ui-post-upgrade @post-upgrade @post-restore @e2e @post-release (requires-ocp/g0) (obs_rbac/g0)",
		func() {
			By("Checking if MCO can be deleted by user1 and admin", func() {
				Eventually(func() error {
					cmd := exec.Command("oc", "policy", "who-can", "delete", "mco")
					var out bytes.Buffer
					cmd.Stdout = &out
					err = cmd.Run()
					if err != nil {
						return err
					}
					if bytes.Contains(out.Bytes(), []byte("user1")) {
						return fmt.Errorf("user1 can delete multiclusterobservabilities.observability.open-cluster-management.io CR")
					}
					if !bytes.Contains(out.Bytes(), []byte("admin")) {
						return fmt.Errorf("admin can't delete multiclusterobservabilities.observability.open-cluster-management.io CR")
					}
					return nil
				}, EventuallyTimeoutMinute*1, EventuallyIntervalSecond*5).Should(Succeed())
			})
		},
	)

	AfterEach(func() {
		if CurrentSpecReport().State.Is(types.SpecStateSkipped) {
			return
		}
		os.Unsetenv("USER_TOKEN")
		if CurrentSpecReport().Failed() {
			utils.LogFailingTestStandardDebugInfo(testOptions, false)
		}
		testFailed = testFailed || CurrentSpecReport().Failed()
	})
})
