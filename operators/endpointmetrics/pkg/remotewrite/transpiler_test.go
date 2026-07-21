package remotewrite

import (
	"strings"
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func convertToPromRelabel(cfgs []monitoringv1.RelabelConfig) []*relabel.Config {
	var ret []*relabel.Config
	for _, c := range cfgs {
		var srcLabels model.LabelNames
		for _, sl := range c.SourceLabels {
			srcLabels = append(srcLabels, model.LabelName(sl))
		}

		re := relabel.MustNewRegexp(c.Regex)

		replacement := "$1"
		if c.Replacement != nil {
			replacement = *c.Replacement
		}

		ret = append(ret, &relabel.Config{
			SourceLabels: srcLabels,
			Separator:    ";",
			Regex:        re,
			TargetLabel:  c.TargetLabel,
			Replacement:  replacement,
			Action:       relabel.Action(strings.ToLower(c.Action)),
		})
	}
	return ret
}

func TestTranspile(t *testing.T) {
	scrapeConfig := &monitoringv1alpha1.ScrapeConfig{
		Spec: monitoringv1alpha1.ScrapeConfigSpec{
			Params: map[string][]string{
				"match[]": {
					"up",
					"workqueue_adds_total{job=\"apiserver\"}",
					"container_memory_cache{container!=\"POD\",container!=\"\"}",
					"process_resident_memory_bytes{job=~\"apiserver|etcd\"}",
				},
			},
			MetricRelabelConfigs: []monitoringv1.RelabelConfig{
				{
					Action: "labeldrop",
					Regex:  "foo",
				},
			},
		},
	}

	agent := &monitoringv1alpha1.PrometheusAgent{
		Spec: monitoringv1alpha1.PrometheusAgentSpec{
			CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
				RemoteWrite: []monitoringv1.RemoteWriteSpec{
					{
						Name:          ptr.To("acm-observability"),
						URL:           "https://example.com/write",
						RemoteTimeout: ptr.To(monitoringv1.Duration("30s")),
						QueueConfig: &monitoringv1.QueueConfig{
							MaxShards: 10,
						},
						TLSConfig: &monitoringv1.TLSConfig{
							SafeTLSConfig: monitoringv1.SafeTLSConfig{
								CA: monitoringv1.SecretOrConfigMap{
									Secret: &corev1.SecretKeySelector{
										Key: "ca.crt",
									},
								},
								InsecureSkipVerify: ptr.To(true),
							},
						},
						Authorization: &monitoringv1.Authorization{
							SafeAuthorization: monitoringv1.SafeAuthorization{
								Type: "Bearer",
							},
						},
					},
				},
			},
		},
	}

	got, err := Transpile(scrapeConfig, agent)
	if err != nil {
		t.Fatalf("Transpile returned error: %v", err)
	}
	if got == nil {
		t.Fatalf("Transpile returned nil")
	}

	// Verify QueueConfig and TLSConfig were copied
	if got.URL != "https://example.com/write" {
		t.Errorf("Expected URL to be copied, got %s", got.URL)
	}
	if got.RemoteTimeout == nil || *got.RemoteTimeout != "30s" {
		t.Errorf("Expected RemoteTimeout to be copied, got %+v", got.RemoteTimeout)
	}
	if got.Authorization == nil || got.Authorization.Type != "Bearer" {
		t.Errorf("Expected Authorization to be copied, got %+v", got.Authorization)
	}
	if got.QueueConfig == nil || got.QueueConfig.MaxShards != 10 {
		t.Errorf("Expected QueueConfig MaxShards to be 10, got %+v", got.QueueConfig)
	}
	if got.TLSConfig == nil || !*got.TLSConfig.InsecureSkipVerify {
		t.Errorf("Expected TLSConfig InsecureSkipVerify to be true, got %+v", got.TLSConfig)
	}

	// Deep behavior test executing the generated relabel configs using Prometheus's own relabeling engine!
	promCfgs := convertToPromRelabel(got.WriteRelabelConfigs)

	tests := []struct {
		name         string
		inputLabels  map[string]string
		expectedKeep bool
	}{
		{
			name:         "Keep standard up metric",
			inputLabels:  map[string]string{"__name__": "up"},
			expectedKeep: true,
		},
		{
			name:         "Keep up metric with arbitrary labels",
			inputLabels:  map[string]string{"__name__": "up", "job": "prometheus"},
			expectedKeep: true,
		},
		{
			name:         "Keep workqueue_adds_total with matching job",
			inputLabels:  map[string]string{"__name__": "workqueue_adds_total", "job": "apiserver"},
			expectedKeep: true,
		},
		{
			name:         "Drop workqueue_adds_total with non-matching job",
			inputLabels:  map[string]string{"__name__": "workqueue_adds_total", "job": "not-apiserver"},
			expectedKeep: false,
		},
		{
			name:         "Keep container_memory_cache with compliant container",
			inputLabels:  map[string]string{"__name__": "container_memory_cache", "container": "main"},
			expectedKeep: true,
		},
		{
			name:         "Drop container_memory_cache with negative container matching POD",
			inputLabels:  map[string]string{"__name__": "container_memory_cache", "container": "POD"},
			expectedKeep: false,
		},
		{
			name:         "Drop container_memory_cache with negative empty container",
			inputLabels:  map[string]string{"__name__": "container_memory_cache", "container": ""},
			expectedKeep: false,
		},
		{
			name:         "Keep process_resident_memory_bytes with apiserver job",
			inputLabels:  map[string]string{"__name__": "process_resident_memory_bytes", "job": "apiserver"},
			expectedKeep: true,
		},
		{
			name:         "Keep process_resident_memory_bytes with etcd job",
			inputLabels:  map[string]string{"__name__": "process_resident_memory_bytes", "job": "etcd"},
			expectedKeep: true,
		},
		{
			name:         "Drop process_resident_memory_bytes with other job",
			inputLabels:  map[string]string{"__name__": "process_resident_memory_bytes", "job": "other"},
			expectedKeep: false,
		},
		{
			name:         "Drop completely unrelated metric (whitelist behavior)",
			inputLabels:  map[string]string{"__name__": "node_cpu_seconds_total"},
			expectedKeep: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := labels.FromMap(tc.inputLabels)
			lb := labels.NewBuilder(input)
			keep := relabel.ProcessBuilder(lb, promCfgs...)

			if keep != tc.expectedKeep {
				t.Fatalf("Expected keep to be %v, got %v for metric %v", tc.expectedKeep, keep, input)
			}

			if keep {
				res := lb.Labels()
				// Verify custom metricRelabelings (the custom labeldrop of label 'foo')
				if res.Get("foo") != "" {
					t.Errorf("Expected label 'foo' to be dropped, but got value '%s'", res.Get("foo"))
				}
			}
		})
	}
}

func TestNegationBugAndNoMetricNameSelectors(t *testing.T) {
	scrapeConfig := &monitoringv1alpha1.ScrapeConfig{
		Spec: monitoringv1alpha1.ScrapeConfigSpec{
			Params: map[string][]string{
				"match[]": {
					// Selector 0 has negative container matcher on container_memory_cache
					"container_memory_cache{container!=\"POD\"}",
					// Selector 1 matches any metric (with or without metric name) having job="prometheus"
					"{job=\"prometheus\"}",
					// Selector 2 matches regex metric names up|down
					"{__name__=~\"up|down\", job=\"apiserver\"}",
					// Selector 3 has multiple positive matchers on the same label 'job'
					"up{job=~\"api.*\", job=~\".*-secure\"}",
				},
			},
		},
	}

	got, err := Transpile(scrapeConfig, nil)
	if err != nil {
		t.Fatalf("Transpile returned error: %v", err)
	}

	promCfgs := convertToPromRelabel(got.WriteRelabelConfigs)

	tests := []struct {
		name         string
		inputLabels  map[string]string
		expectedKeep bool
	}{
		{
			// Resolves high priority bug 1: The Negation Bug (cross-selector drops)
			// Selector 0 would drop container="POD", but Selector 1 matches it due to job="prometheus".
			// In standard Prometheus disjunction (OR) rules, this metric MUST be kept!
			name:         "Negation Bug check: Keep metric matching Selector 1 despite matching Selector 0's negative filter",
			inputLabels:  map[string]string{"__name__": "container_memory_cache", "container": "POD", "job": "prometheus"},
			expectedKeep: true,
		},
		{
			// Resolves high priority bug 2: Selectors without exact metric names
			// Selector 1 matches any metric name (e.g., "my_custom_metric") as long as job="prometheus".
			name:         "Keep custom metric without standard name matching Selector 1",
			inputLabels:  map[string]string{"__name__": "my_custom_metric", "job": "prometheus"},
			expectedKeep: true,
		},
		{
			// Resolves high priority bug 2: Regex metric names
			// Selector 2 matches up{job="apiserver"}
			name:         "Keep up metric matching Selector 2",
			inputLabels:  map[string]string{"__name__": "up", "job": "apiserver"},
			expectedKeep: true,
		},
		{
			// Selector 2 matches down{job="apiserver"}
			name:         "Keep down metric matching Selector 2",
			inputLabels:  map[string]string{"__name__": "down", "job": "apiserver"},
			expectedKeep: true,
		},
		{
			// Selector 2 does not match other{job="apiserver"}
			name:         "Drop other metric not matching Selector 2",
			inputLabels:  map[string]string{"__name__": "other", "job": "apiserver"},
			expectedKeep: false,
		},
		{
			// Selector 3 matches up{job="api-secure"} (both job=~"api.*" and job=~".*-secure" are satisfied)
			name:         "Keep up metric matching multiple positive matchers on the same label",
			inputLabels:  map[string]string{"__name__": "up", "job": "api-secure"},
			expectedKeep: true,
		},
		{
			// Selector 3 does not match up{job="api-insecure"} (job=~".*-secure" is not satisfied)
			name:         "Drop up metric matching only one positive matcher on the same label",
			inputLabels:  map[string]string{"__name__": "up", "job": "api-insecure"},
			expectedKeep: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := labels.FromMap(tc.inputLabels)
			lb := labels.NewBuilder(input)
			keep := relabel.ProcessBuilder(lb, promCfgs...)

			if keep != tc.expectedKeep {
				for _, c := range got.WriteRelabelConfigs {
					repl := ""
					if c.Replacement != nil {
						repl = *c.Replacement
					}
					t.Logf("Config: Action=%s, SourceLabels=%v, Regex=%s, TargetLabel=%s, Replacement=%s", c.Action, c.SourceLabels, c.Regex, c.TargetLabel, repl)
				}
				t.Fatalf("Expected keep to be %v, got %v for metric %v", tc.expectedKeep, keep, input)
			}
		})
	}
}

func TestSharedPointerCacheIsolation(t *testing.T) {
	scrapeConfig := &monitoringv1alpha1.ScrapeConfig{
		Spec: monitoringv1alpha1.ScrapeConfigSpec{
			Params: map[string][]string{
				"match[]": {"up"},
			},
		},
	}

	originalAgent := &monitoringv1alpha1.PrometheusAgent{
		Spec: monitoringv1alpha1.PrometheusAgentSpec{
			CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
				RemoteWrite: []monitoringv1.RemoteWriteSpec{
					{
						Name:          ptr.To("acm-observability"),
						URL:           "https://example.com/write",
						RemoteTimeout: ptr.To(monitoringv1.Duration("30s")),
						QueueConfig: &monitoringv1.QueueConfig{
							MaxShards: 10,
						},
						TLSConfig: &monitoringv1.TLSConfig{
							SafeTLSConfig: monitoringv1.SafeTLSConfig{
								InsecureSkipVerify: ptr.To(true),
							},
						},
						Authorization: &monitoringv1.Authorization{
							SafeAuthorization: monitoringv1.SafeAuthorization{
								Type: "Bearer",
							},
						},
						Headers: map[string]string{
							"X-Hub": "ACM",
						},
					},
				},
			},
		},
	}

	got, err := Transpile(scrapeConfig, originalAgent)
	if err != nil {
		t.Fatalf("Transpile returned error: %v", err)
	}

	// Downstream mutation of the generated RemoteWriteSpec pointers/maps
	got.QueueConfig.MaxShards = 99
	*got.TLSConfig.InsecureSkipVerify = false
	*got.RemoteTimeout = "99s"
	got.Authorization.Type = "Basic"
	got.Headers["X-Hub"] = "Mutated"

	// Assert that the original agent's values are perfectly untouched
	// (Resolves high priority bug 3: Shared Pointer and Reference Mutation Risk)
	if originalAgent.Spec.RemoteWrite[0].QueueConfig.MaxShards != 10 {
		t.Errorf("Memory leak/aliasing detected! Original agent QueueConfig was mutated to %d", originalAgent.Spec.RemoteWrite[0].QueueConfig.MaxShards)
	}
	if !*originalAgent.Spec.RemoteWrite[0].TLSConfig.InsecureSkipVerify {
		t.Errorf("Memory leak/aliasing detected! Original agent TLSConfig was mutated")
	}
	if *originalAgent.Spec.RemoteWrite[0].RemoteTimeout != "30s" {
		t.Errorf("Memory leak/aliasing detected! Original agent RemoteTimeout was mutated")
	}
	if originalAgent.Spec.RemoteWrite[0].Authorization.Type != "Bearer" {
		t.Errorf("Memory leak/aliasing detected! Original agent Authorization was mutated to %s", originalAgent.Spec.RemoteWrite[0].Authorization.Type)
	}
	if originalAgent.Spec.RemoteWrite[0].Headers["X-Hub"] != "ACM" {
		t.Errorf("Memory leak/aliasing detected! Original agent Headers map was mutated")
	}
}
