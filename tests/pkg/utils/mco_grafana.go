// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	"k8s.io/klog"
)

var (
	testHeadless bool

	BearerToken             string
	baseDomain              string
	kubeadminUser           string
	kubeadminCredential     string
	kubeconfig              string
	reportFile              string
	optionsFile             string
	ownerPrefix, ocpRelease string

	testOptions          TestOptions
	testOptionsContainer TestOptionsContainer
	testUITimeout        time.Duration

	testFailed = false
)

func GetGrafanaURL(opt TestOptions) string {
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
	cloudProvider := strings.ToLower(os.Getenv("CLOUD_PROVIDER"))
	substring1 := "rosa"
	substring2 := "hcp"
	if strings.Contains(cloudProvider, substring1) && strings.Contains(cloudProvider, substring2) {

		grafanaConsoleURL := "https://grafana-open-cluster-management-observability.apps.rosa." + opt.HubCluster.BaseDomain
		if opt.HubCluster.GrafanaURL != "" {
			grafanaConsoleURL = opt.HubCluster.GrafanaURL
		} else {
			opt.HubCluster.GrafanaHost = "grafana-open-cluster-management-observability.apps.rosa." + opt.HubCluster.BaseDomain
		}
		return grafanaConsoleURL
	} else {
		grafanaConsoleURL := "https://grafana-open-cluster-management-observability.apps." + opt.HubCluster.BaseDomain
		if opt.HubCluster.GrafanaURL != "" {
			grafanaConsoleURL = opt.HubCluster.GrafanaURL
		} else {
			opt.HubCluster.GrafanaHost = "grafana-open-cluster-management-observability.apps." + opt.HubCluster.BaseDomain
		}
		return grafanaConsoleURL
	}
}
