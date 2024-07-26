package microshift

import (
	"os"
	"testing"

	"github.com/go-logr/logr"
	promcfg "github.com/prometheus/prometheus/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func TestScrapeConfigUpdate(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	secretFile, err := os.ReadFile("../../manifests/prometheus/prometheus-scrape-targets-secret.yaml")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}

	secret := &corev1.Secret{}
	if err := yaml.Unmarshal(secretFile, secret); err != nil {
		t.Fatalf("Failed to unmarshal secret: %v", err)
	}

	// Transform secret to unstructured
	secretUnstructured, err := convertToUnstructured(secret)
	if err != nil {
		t.Fatalf("Failed to convert secret to unstructured: %v", err)
	}

	mc := NewMicroshift(client, "ns", logr.Logger{})
	resources := []*unstructured.Unstructured{secretUnstructured}
	if err := mc.renderScrapeConfig(resources); err != nil {
		t.Fatalf("Failed to render scrape config: %v", err)
	}

	newScrapeSecret := &corev1.Secret{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(secretUnstructured.Object, newScrapeSecret); err != nil {
		t.Fatalf("Failed to convert unstructured to secret: %v", err)
	}

	sc := ScrapeConfigs{}
	if err := sc.UnmarshalYAML([]byte(newScrapeSecret.StringData[scrapeConfigKey])); err != nil {
		t.Fatalf("Failed to unmarshal scrape config: %v", err)
	}

	// Check if the scrape config has been updated
	assert.Greater(t, len(sc.ScrapeConfigs), 1, "Scrape config not updated")
	found := false
	for _, scrapeConfig := range sc.ScrapeConfigs {
		if scrapeConfig.JobName == "etcd" {
			found = true
			break
		}
	}
	assert.True(t, found, "Scrape config not updated")

	// validate configs
	for _, scrapeConfig := range sc.ScrapeConfigs {
		assert.NoError(t, scrapeConfig.Validate(promcfg.DefaultGlobalConfig))
		assert.NoError(t, scrapeConfig.HTTPClientConfig.Validate())
	}
}
