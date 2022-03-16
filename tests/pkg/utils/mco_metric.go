// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
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
			/* #nosec */
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
		return fmt.Errorf("Failed to access managed cluster metrics via grafana console"), false
	}

	metricResult, err := ioutil.ReadAll(resp.Body)
	klog.V(5).Infof("metricResult: %s\n", metricResult)
	if err != nil {
		return err, false
	}

	if !strings.Contains(string(metricResult), `"status":"success"`) {
		return fmt.Errorf("Failed to find valid status from response for query: %s", query), false
	}

	if strings.Contains(string(metricResult), `"result":[]`) {
		return fmt.Errorf("Failed to find metric name from response for query: %s", query), false
	}

	contained := true
	for _, label := range matchedLabels {
		if !strings.Contains(string(metricResult), label) {
			contained = false
			break
		}
	}
	if !contained {
		return fmt.Errorf("Failed to find metric name from response"), false
	}

	return nil, true
}

type MetricsAllowlist struct {
	NameList  []string          `yaml:"names"`
	MatchList []string          `yaml:"matches"`
	RenameMap map[string]string `yaml:"renames"`
	RuleList  []Rule            `yaml:"rules"`
}

// Rule is the struct for recording rules and alert rules
type Rule struct {
	Record string `yaml:"record"`
	Expr   string `yaml:"expr"`
}

func GetDefaultMetricList(opt TestOptions) []string {
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
		klog.Errorf("Failed to unmarshal data: %+v\n", err)
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
	return allDefaultMetricName
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
