// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	"k8s.io/klog"

	"github.com/open-cluster-management/observability-e2e-test/pkg/utils"
)

//var bareBaseDomain string
var baseDomain string
var kubeadminUser string
var kubeadminCredential string
var kubeconfig string
var reportFile string

var registry string
var registryUser string
var registryPassword string

var optionsFile, clusterDeployFile, installConfigFile string
var testOptions utils.TestOptions
var clusterDeploy utils.ClusterDeploy
var installConfig utils.InstallConfig
var testOptionsContainer utils.TestOptionsContainer
var testUITimeout time.Duration
var testHeadless bool

var ownerPrefix string

var hubNamespace string
var pullSecretName string
var installConfigAWS, installConfigGCP, installConfigAzure string
var hiveClusterName, hiveGCPClusterName, hiveAzureClusterName string

var ocpRelease string

var testFailed = false

const OCP_RELEASE_DEFAULT = "4.4.4"

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"0123456789"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func randString(length int) string {
	return StringWithCharset(length, charset)
}

func init() {
	klog.SetOutput(GinkgoWriter)
	klog.InitFlags(nil)

	flag.StringVar(&kubeadminUser, "kubeadmin-user", "kubeadmin", "Provide the kubeadmin credential for the cluster under test (e.g. -kubeadmin-user=\"xxxxx\").")
	flag.StringVar(&kubeadminCredential, "kubeadmin-credential", "", "Provide the kubeadmin credential for the cluster under test (e.g. -kubeadmin-credential=\"xxxxx-xxxxx-xxxxx-xxxxx\").")
	flag.StringVar(&baseDomain, "base-domain", "", "Provide the base domain for the cluster under test (e.g. -base-domain=\"demo.red-chesterfield.com\").")
	flag.StringVar(&reportFile, "report-file", "results.xml", "Provide the path to where the junit results will be printed.")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Location of the kubeconfig to use; defaults to KUBECONFIG if not set")
	flag.StringVar(&optionsFile, "options", "", "Location of an \"options.yaml\" file to provide input for various tests")
}

func TestObservabilityE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	junitReporter := reporters.NewJUnitReporter(reportFile)
	RunSpecsWithDefaultAndCustomReporters(t, "Observability E2E Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	initVars()
	installMCO()
})

var _ = AfterSuite(func() {
	if !testFailed {
		uninstallMCO()
	} else {
		utils.PrintAllMCOPodsStatus(testOptions)
	}
})

func initVars() {

	// default ginkgo test timeout 30s
	// increased from original 10s
	testUITimeout = time.Second * 30

	if optionsFile == "" {
		optionsFile = os.Getenv("OPTIONS")
		if optionsFile == "" {
			optionsFile = "resources/options.yaml"
		}
	}

	klog.V(1).Infof("options filename=%s", optionsFile)

	data, err := ioutil.ReadFile(optionsFile)
	if err != nil {
		klog.Errorf("--options error: %v", err)
	}
	Expect(err).NotTo(HaveOccurred())

	fmt.Printf("file preview: %s \n", string(optionsFile))

	err = yaml.Unmarshal([]byte(data), &testOptionsContainer)
	if err != nil {
		klog.Errorf("--options error: %v", err)
	}

	testOptions = testOptionsContainer.Options

	// default Headless is `true`
	// to disable, set Headless: false
	// in options file
	if testOptions.Headless == "" {
		testHeadless = true
	} else {
		if testOptions.Headless == "false" {
			testHeadless = false
		} else {
			testHeadless = true
		}
	}

	// OwnerPrefix is used to help identify who owns deployed resources
	//    If a value is not supplied, the default is OS environment variable $USER
	if testOptions.OwnerPrefix == "" {
		ownerPrefix = os.Getenv("USER")
		if ownerPrefix == "" {
			ownerPrefix = "ginkgo"
		}
	} else {
		ownerPrefix = testOptions.OwnerPrefix
	}
	klog.V(1).Infof("ownerPrefix=%s", ownerPrefix)

	if testOptions.Connection.OCPRelease == "" {
		ocpRelease = OCP_RELEASE_DEFAULT
	} else {
		ocpRelease = testOptions.Connection.OCPRelease
	}
	klog.V(1).Infof("ocpRelease=%s", ocpRelease)

	if testOptions.KubeConfig == "" {
		if kubeconfig == "" {
			kubeconfig = os.Getenv("KUBECONFIG")
		}
		testOptions.KubeConfig = kubeconfig
	}

	if testOptions.HubCluster.BaseDomain != "" {
		baseDomain = testOptions.HubCluster.BaseDomain

		if testOptions.HubCluster.MasterURL == "" {
			testOptions.HubCluster.MasterURL = fmt.Sprintf("https://api.%s:6443", testOptions.HubCluster.BaseDomain)
		}
	} else {
		Expect(baseDomain).NotTo(BeEmpty(), "The `baseDomain` is required.")
		testOptions.HubCluster.BaseDomain = baseDomain
		testOptions.HubCluster.MasterURL = fmt.Sprintf("https://api.%s:6443", baseDomain)
	}

	if testOptions.HubCluster.User != "" {
		kubeadminUser = testOptions.HubCluster.User
	}
	if testOptions.HubCluster.Password != "" {
		kubeadminCredential = testOptions.HubCluster.Password
	}

	if testOptions.ManagedClusters != nil && len(testOptions.ManagedClusters) > 0 {
		for i, mc := range testOptions.ManagedClusters {
			if mc.MasterURL == "" {
				testOptions.ManagedClusters[i].MasterURL = fmt.Sprintf("https://api.%s:6443", mc.BaseDomain)
			}
			if mc.KubeConfig == "" {
				testOptions.ManagedClusters[i].KubeConfig = os.Getenv("IMPORT_KUBECONFIG")
			}
		}
	}
}
