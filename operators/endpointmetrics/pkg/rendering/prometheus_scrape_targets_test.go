// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"os"
	"path"
	"testing"

	"gopkg.in/yaml.v2"
)

// kubeletScrapeJobs maps each kubelet scrape job to the metrics path it must
// scrape. These jobs are deployed on non-OCP managed clusters only, where ACM
// runs its own Prometheus instead of federating from openshift-monitoring.
var kubeletScrapeJobs = map[string]string{
	"kubelet-metrics":          "/metrics",
	"kubelet-metrics-cadvisor": "/metrics/cadvisor",
	"kubelet-metrics-probes":   "/metrics/probes",
}

type scrapeRelabelConfig struct {
	Action       string   `yaml:"action"`
	SourceLabels []string `yaml:"source_labels"`
	Regex        string   `yaml:"regex"`
	TargetLabel  string   `yaml:"target_label"`
	Replacement  string   `yaml:"replacement"`
}

type scrapeJob struct {
	JobName        string                `yaml:"job_name"`
	MetricsPath    string                `yaml:"metrics_path"`
	RelabelConfigs []scrapeRelabelConfig `yaml:"relabel_configs"`
}

// TestKubeletScrapeJobsDoNotOverrideAddress guards against the regression
// introduced by ACM-29127 (PR #2342), where the kubelet scrape jobs overwrote
// the __address__ computed by `kubernetes_sd_configs: role: node` with
// <node-name>:10250. Node names are not resolvable through cluster DNS on many
// clusters (AKS node names are VMSS instance names, Kind nodes are container
// names, and most bare-metal installs use hostnames that are not in DNS), so
// every kubelet scrape failed with "no such host" and kubelet, cAdvisor and
// probe metrics stopped reaching the hub (ACM-49893).
//
// role: node already sets __address__ to "<node address>:<kubelet port>" using
// net.JoinHostPort (so IPv6 addresses are bracketed) and honouring
// status.daemonEndpoints.kubeletEndpoint.port. Overriding it is never correct,
// hence asserting on the manifest, which is the artifact that ships.
func TestKubeletScrapeJobsDoNotOverrideAddress(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	manifestPath := path.Join(
		path.Dir(path.Dir(wd)), "manifests", "prometheus", "prometheus-scrape-targets-secret.yaml",
	)

	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", manifestPath, err)
	}

	var secret struct {
		StringData map[string]string `yaml:"stringData"`
	}
	if err := yaml.Unmarshal(rawManifest, &secret); err != nil {
		t.Fatalf("failed to parse %s: %v", manifestPath, err)
	}

	scrapeConfig, ok := secret.StringData["scrape-targets.yaml"]
	if !ok {
		t.Fatalf("%s does not contain the scrape-targets.yaml key", manifestPath)
	}

	var jobs []scrapeJob
	if err := yaml.Unmarshal([]byte(scrapeConfig), &jobs); err != nil {
		t.Fatalf("failed to parse scrape-targets.yaml: %v", err)
	}

	found := map[string]bool{}
	for _, job := range jobs {
		wantMetricsPath, isKubeletJob := kubeletScrapeJobs[job.JobName]
		if !isKubeletJob {
			continue
		}
		found[job.JobName] = true

		var addressReplacement, nodeReplacement, metricsPathReplacement string
		for _, relabel := range job.RelabelConfigs {
			switch relabel.TargetLabel {
			case "__address__":
				addressReplacement = relabel.Replacement
			case "node":
				nodeReplacement = relabel.Replacement
			case "__metrics_path__":
				metricsPathReplacement = relabel.Replacement
			}
		}

		if addressReplacement != "" {
			t.Errorf("job %q overrides __address__ with %q: the kubelet address must come from "+
				"kubernetes node service discovery, otherwise scrapes fail on clusters whose node "+
				"names are not resolvable through DNS (ACM-49893)",
				job.JobName, addressReplacement)
		}
		if nodeReplacement == "" {
			t.Errorf("job %q no longer sets the node label from __meta_kubernetes_node_name", job.JobName)
		}
		if metricsPathReplacement != wantMetricsPath {
			t.Errorf("job %q scrapes %q, expected %q",
				job.JobName, metricsPathReplacement, wantMetricsPath)
		}
	}

	for jobName := range kubeletScrapeJobs {
		if !found[jobName] {
			t.Errorf("job %q is missing from scrape-targets.yaml", jobName)
		}
	}
}
