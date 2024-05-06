// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func ContainManagedClusterMetric(opt TestOptions, query string, matchedLabels []string) (error, bool) {
	grafanaConsoleURL := GetGrafanaURL(opt)
	path := "/api/datasources/proxy/1/api/v1/query?"
	queryParams := url.PathEscape(fmt.Sprintf("query=%s", query))
	klog.V(5).Infof("request url is: %s\n", grafanaConsoleURL+path+queryParams)
	req, err := http.NewRequest(
		"GET",
		grafanaConsoleURL+path+queryParams,
		nil)
	if err != nil {
		return err, false
	}

	client := &http.Client{}
	if os.Getenv("IS_KIND_ENV") != "true" {
		tr := &http.Transport{
			// #nosec G402 -- Only used in test.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		client = &http.Client{Transport: tr}
		token, err := FetchBearerToken(opt)
		if err != nil {
			return err, false
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Host = opt.HubCluster.GrafanaHost
	}

	resp, err := client.Do(req)
	if err != nil {
		return err, false
	}

	if resp.StatusCode != http.StatusOK {
		klog.Errorf("resp: %+v\n", resp)
		klog.Errorf("err: %+v\n", err)
		return fmt.Errorf("failed to access managed cluster metrics via grafana console: %s", query), false
	}

	metricResult, err := io.ReadAll(resp.Body)
	klog.V(5).Infof("metricResult: %s\n", metricResult)
	if err != nil {
		klog.V(5).Info("metricResult err: \n")
		return err, false
	}

	if !strings.Contains(string(metricResult), `"status":"success"`) {
		klog.V(5).Info("metric doesn't contain status success\n")
		return errors.New("failed to find valid status from response"), false
	}

	if strings.Contains(string(metricResult), `"result":[]`) {
		klog.V(5).Info("Found empty result")
		return errors.New("failed to find metric name from response"), false
	}

	contained := true
	for _, label := range matchedLabels {
		if !strings.Contains(string(metricResult), label) {
			klog.V(5).Infof("Didn't find: %s, in metrics result", label)
			contained = false
			break
		}
	}
	if !contained {
		return errors.New("failed to find metric name from response"), false
	}

	return nil, true
}

type MetricsAllowlist struct {
	NameList             []string           `yaml:"names"`
	MatchList            []string           `yaml:"matches"`
	RenameMap            map[string]string  `yaml:"renames"`
	RuleList             []RecordingRule    `yaml:"rules"` //deprecated
	RecordingRuleList    []RecordingRule    `yaml:"recording_rules"`
	CollectRuleGroupList []CollectRuleGroup `yaml:"collect_rules"`
}

type RecordingRule struct {
	Record string `yaml:"record"`
	Expr   string `yaml:"expr"`
}
type CollectRule struct {
	Collect     string            `yaml:"collect"`
	Annotations map[string]string `yaml:"annotations"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Metrics     DynamicMetrics    `yaml:"dynamic_metrics"`
}

type DynamicMetrics struct {
	NameList  []string `yaml:"names"`
	MatchList []string `yaml:"matches"`
}

type CollectRuleSelector struct {
	MatchExpression []metav1.LabelSelectorRequirement `yaml:"matchExpressions"`
}

// CollectRuleGroup structure contains information of a group of collect rules used for
// dnamically collecting metrics.
type CollectRuleGroup struct {
	Name            string              `yaml:"group"`
	Annotations     map[string]string   `yaml:"annotations"`
	Selector        CollectRuleSelector `yaml:"selector"`
	CollectRuleList []CollectRule       `yaml:"rules"`
}

func GetDefaultMetricList(opt TestOptions) ([]string, []string) {
	allDefaultMetricName := []string{}
	cl := getKubeClient(opt, true)
	cm, err := cl.CoreV1().ConfigMaps(MCO_NAMESPACE).Get(
		context.TODO(),
		"observability-metrics-allowlist",
		metav1.GetOptions{},
	)
	if err != nil {
		klog.Errorf("Failed to get the configmap <%v>: %+v\n",
			"observability-metrics-allowlist",
			err)
	}

	allowlist := &MetricsAllowlist{}
	err = yaml.Unmarshal([]byte(cm.Data["metrics_list.yaml"]), allowlist)
	if err != nil {
		klog.Errorf("failed to unmarshal data: %+v\n", err)
	}

	allDefaultMetricName = append(allDefaultMetricName, allowlist.NameList...)

	// get the metric name from matches section:
	// string: __name__="go_goroutines",job="apiserver"
	// want: go_goroutines
	re := regexp.MustCompile("__name__=\"(\\w+)\"")
	for _, name := range allowlist.MatchList {
		result := re.FindStringSubmatch(name)
		if len(result) > 1 {
			allDefaultMetricName = append(allDefaultMetricName, result[1])
		}
	}

	for _, name := range allowlist.RenameMap {
		allDefaultMetricName = append(allDefaultMetricName, name)
	}

	for _, rule := range allowlist.RuleList {
		allDefaultMetricName = append(allDefaultMetricName, rule.Record)
	}

	dynamicMetricsName := []string{}
	for _, collectRuleGroups := range allowlist.CollectRuleGroupList {
		for _, collectRule := range collectRuleGroups.CollectRuleList {
			dynamicMetricsName = append(dynamicMetricsName, collectRule.Metrics.NameList...)
			for _, match := range collectRule.Metrics.MatchList {
				if name := getNameInMatch(match); name != "" {
					dynamicMetricsName = append(dynamicMetricsName, name)
				}
			}
		}
	}

	return allDefaultMetricName, dynamicMetricsName
}

func GetIgnoreMetricMap() map[string]bool {
	txtlines := map[string]bool{}
	file, err := os.Open("../testdata/ignored-metric-list")
	if err != nil {
		klog.Errorf("failed to open the ignored-metric-list file: %+v\n", err)
	}

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		txtlines[scanner.Text()] = true
	}
	return txtlines
}

func getNameInMatch(match string) string {
	r := regexp.MustCompile(`__name__="([^,]*)"`)
	m := r.FindAllStringSubmatch(match, -1)
	if m != nil {
		return m[0][1]
	}
	return ""
}
