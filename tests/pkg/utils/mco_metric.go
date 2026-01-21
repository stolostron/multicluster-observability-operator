// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type GrafanaResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"` // Use interface{} because value can be mixed types
		} `json:"result"`
	} `json:"data"`
}

func (r GrafanaResponse) ContainsLabelsSet(labels map[string]string) bool {
	ret := false
loop:
	for _, result := range r.Data.Result {
		for key, val := range labels {
			if result.Metric[key] != val {
				continue loop
			}
		}
		ret = true
		break
	}

	return ret
}

func (r GrafanaResponse) CheckMetricFromAllClusters(clusters []Cluster) error {
	for _, cluster := range clusters {
		found := false
		for _, result := range r.Data.Result {
			if result.Metric["cluster"] == cluster.Name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("metric not found for cluster %s", cluster.Name)
		}
	}
	return nil
}

func (r GrafanaResponse) String() string {
	var ret strings.Builder
	ret.WriteString(fmt.Sprintf("Status: %s\n", r.Status))
	ret.WriteString(fmt.Sprintf("ResultType: %s\n", r.Data.ResultType))
	ret.WriteString("Result:\n")
	for _, result := range r.Data.Result {
		ret.WriteString(fmt.Sprintf("%v %v\n", result.Metric, result.Value))
	}
	return ret.String()
}

func QueryGrafana(opt TestOptions, query string) (*GrafanaResponse, error) {
	grafanaConsoleURL := GetGrafanaURL(opt)
	path := "/api/datasources/proxy/uid/000000001/api/v1/query?"
	queryParams := url.PathEscape(fmt.Sprintf("query=%s", query))
	req, err := http.NewRequest(
		http.MethodGet,
		grafanaConsoleURL+path+queryParams,
		nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	if os.Getenv("IS_KIND_ENV") != "true" {
		tr := &http.Transport{
			// #nosec G402 -- Only used in test.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		client = &http.Client{Transport: tr}
		if os.Getenv("USER_TOKEN") != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", strings.TrimSpace(os.Getenv("USER_TOKEN"))))
		} else {
			token, err := FetchBearerToken(opt)
			if err != nil {
				return nil, err
			}

			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}
		req.Host = opt.HubCluster.GrafanaHost
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to access managed cluster metrics via grafana console, status code: %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	metricResult := GrafanaResponse{}
	err = yaml.Unmarshal(respBody, &metricResult)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %v", err)
	}

	if metricResult.Status != "success" {
		return &metricResult, fmt.Errorf("failed to get metric from response, status: %s", metricResult.Status)
	}

	return &metricResult, nil
}

type MetricsAllowlist struct {
	NameList             []string           `yaml:"names"`
	MatchList            []string           `yaml:"matches"`
	RenameMap            map[string]string  `yaml:"renames"`
	RuleList             []RecordingRule    `yaml:"rules"` // deprecated
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
	cl := GetKubeClient(opt, true)
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

func getNameInMatch(match string) string {
	r := regexp.MustCompile(`__name__="([^,]*)"`)
	m := r.FindAllStringSubmatch(match, -1)
	if m != nil {
		return m[0][1]
	}
	return ""
}
