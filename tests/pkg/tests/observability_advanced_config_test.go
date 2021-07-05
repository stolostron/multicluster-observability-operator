// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	"github.com/open-cluster-management/observability-e2e-test/pkg/utils"
)

var _ = Describe("Observability:", func() {
	BeforeEach(func() {
		hubClient = utils.NewKubeClient(
			testOptions.HubCluster.MasterURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)

		dynClient = utils.NewKubeClientDynamic(
			testOptions.HubCluster.MasterURL,
			testOptions.KubeConfig,
			testOptions.HubCluster.KubeContext)
	})

	componentMap := map[string]struct {
		// deployment or statefulset
		Type string
		Name string
	}{
		"alertmanager": {
			Type: "Statefulset",
			Name: MCO_CR_NAME + "-alertmanager",
		},
		"grafana": {
			Type: "Deployment",
			Name: MCO_CR_NAME + "-grafana",
		},
		"observatoriumAPI": {
			Type: "Deployment",
			Name: MCO_CR_NAME + "-observatorium-api",
		},
		"rbacQueryProxy": {
			Type: "Deployment",
			Name: MCO_CR_NAME + "-rbac-query-proxy",
		},
		"compact": {
			Type: "Statefulset",
			Name: MCO_CR_NAME + "-thanos-compact",
		},
		"query": {
			Type: "Deployment",
			Name: MCO_CR_NAME + "-thanos-query",
		},
		"queryFrontend": {
			Type: "Deployment",
			Name: MCO_CR_NAME + "-thanos-query-frontend",
		},
		"queryFrontendMemcached": {
			Type: "Statefulset",
			Name: MCO_CR_NAME + "-thanos-query-frontend-memcached",
		},
		"receive": {
			Type: "Statefulset",
			Name: MCO_CR_NAME + "-thanos-receive-default",
		},
		"rule": {
			Type: "Statefulset",
			Name: MCO_CR_NAME + "-thanos-rule",
		},
		"storeMemcached": {
			Type: "Statefulset",
			Name: MCO_CR_NAME + "-thanos-store-memcached",
		},
		"store": {
			Type: "Statefulset",
			Name: MCO_CR_NAME + "-thanos-store-shard-0",
		},
	}

	It("[P1][Sev1][Observability][Integration] Checking replicas in advanced config for each component (config/g0)", func() {

		mcoRes, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Get(MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}
		advancedSpec := mcoRes.Object["spec"].(map[string]interface{})["advanced"].(map[string]interface{})

		for key, component := range componentMap {
			if key == "compact" || key == "store" {
				continue
			}
			klog.V(1).Infof("The component is: %s\n", key)
			replicas := advancedSpec[key].(map[string]interface{})["replicas"]
			if component.Type == "Deployment" {
				err, deploy := utils.GetDeployment(testOptions, true, component.Name, MCO_NAMESPACE)
				Expect(err).NotTo(HaveOccurred())
				Expect(int(replicas.(int64))).To(Equal(int(*deploy.Spec.Replicas)))
			} else {
				err, sts := utils.GetStatefulSet(testOptions, true, component.Name, MCO_NAMESPACE)
				Expect(err).NotTo(HaveOccurred())
				Expect(int(replicas.(int64))).To(Equal(int(*sts.Spec.Replicas)))
			}
		}
	})

	It("[P2][Sev2][Observability][Integration] Checking resources in advanced config (config/g0)", func() {
		mcoRes, err := dynClient.Resource(utils.NewMCOGVRV1BETA2()).Get(MCO_CR_NAME, metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
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
				err, deploy := utils.GetDeployment(testOptions, true, component.Name, MCO_NAMESPACE)
				Expect(err).NotTo(HaveOccurred())
				Expect(cpu).To(Equal(deploy.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String()))
				Expect(limits["memory"]).To(Equal(deploy.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()))
			} else {
				err, sts := utils.GetStatefulSet(testOptions, true, component.Name, MCO_NAMESPACE)
				Expect(err).NotTo(HaveOccurred())
				Expect(cpu).To(Equal(sts.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String()))
				Expect(limits["memory"]).To(Equal(sts.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()))
			}
		}
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			utils.PrintMCOObject(testOptions)
			utils.PrintAllMCOPodsStatus(testOptions)
			utils.PrintAllOBAPodsStatus(testOptions)
		} else {
			Expect(utils.IntegrityChecking(testOptions)).NotTo(HaveOccurred())
		}
		testFailed = testFailed || CurrentGinkgoTestDescription().Failed
	})
})
