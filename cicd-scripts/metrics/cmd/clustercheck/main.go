// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

/*
Developer debug tool that provides a simple CLI to ensure that a list of metrics used in dashboards is effectively collected
and available for each managed cluster in the Prometheus endpoint.
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stolostron/multicluster-observability-operator/cicd-scripts/metrics/internal/scrapeconfig"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	scrapeConfigsArg := flag.String("scrape-configs", "", "Path to the comma separated scrape_configs")
	prometheusURLArg := flag.String("prometheus-url", "", "URL of the Prometheus/Thanos endpoint")
	ignoredScrapeConfigMetricsArg := flag.String(
		"ignored-scrapeconfig-metrics",
		"",
		"Comma separated ignored ScrapeConfig metrics.",
	)
	chunkSizeArg := flag.Int("chunk-size", 50, "Number of metrics to query in a single request")
	clusterRegexArg := flag.String("cluster-regex", "", "Regex to filter managed clusters by name")
	flag.Parse()

	if *chunkSizeArg <= 0 {
		return fmt.Errorf("chunk-size must be greater than 0")
	}

	fmt.Println("Clustercheck — Verifying platform metrics availability across managed clusters.")

	if *scrapeConfigsArg == "" {
		return fmt.Errorf("please provide the scrape_configs paths")
	}
	if *prometheusURLArg == "" {
		return fmt.Errorf("please provide the prometheus-url")
	}

	ignoredScrapeConfigMetrics := strings.Split(*ignoredScrapeConfigMetricsArg, ",")
	ignoredMetricsMap := make(map[string]struct{}, len(ignoredScrapeConfigMetrics))
	for _, m := range ignoredScrapeConfigMetrics {
		if m != "" {
			ignoredMetricsMap[m] = struct{}{}
		}
	}

	collectedMetrics, err := scrapeconfig.ReadFederatedMetrics(*scrapeConfigsArg)
	if err != nil {
		return fmt.Errorf("failed to read scrape configs: %w", err)
	}

	collectedMetrics = slices.DeleteFunc(collectedMetrics, func(s string) bool {
		if s == "" {
			return true
		}
		_, ignored := ignoredMetricsMap[s]
		return ignored
	})

	// Remove duplicates from collected metrics using built-ins
	slices.Sort(collectedMetrics)
	collectedMetrics = slices.Compact(collectedMetrics)

	if len(collectedMetrics) == 0 {
		return fmt.Errorf("no metrics found in the provided scrape configs")
	}

	// Set up Prometheus client
	client, err := api.NewClient(api.Config{
		Address: *prometheusURLArg,
	})
	if err != nil {
		return fmt.Errorf("error creating Prometheus client: %w", err)
	}
	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Get list of managed clusters
	clusters, err := getManagedClusters(ctx, v1api, *clusterRegexArg)
	if err != nil {
		return fmt.Errorf("error fetching managed clusters: %w", err)
	}
	if len(clusters) == 0 {
		fmt.Println("No managed clusters found.")
		return nil
	}
	fmt.Printf("Discovered %d managed clusters: %s\n", len(clusters), strings.Join(clusters, ", "))

	// 2. Fetch metric availability
	clusterMetricsPresent, err := fetchMetricAvailability(ctx, v1api, clusters, collectedMetrics, *chunkSizeArg)
	if err != nil {
		return fmt.Errorf("error fetching metric availability: %w", err)
	}

	// 3. Report Generation
	var missingAllClusters []string
	for _, metric := range collectedMetrics {
		missingInAll := true
		for _, cluster := range clusters {
			if clusterMetricsPresent[cluster][metric] {
				missingInAll = false
				break
			}
		}
		if missingInAll {
			missingAllClusters = append(missingAllClusters, metric)
		}
	}
	sort.Strings(missingAllClusters)

	allSuccess := true
	clustersWithMissingMetrics := 0
	for _, cluster := range clusters {
		var missing []string
		var present []string

		for _, metric := range collectedMetrics {
			if clusterMetricsPresent[cluster][metric] {
				present = append(present, metric)
			} else {
				missing = append(missing, metric)
			}
		}

		sort.Strings(missing)
		sort.Strings(present)

		if len(missing) > 0 {
			allSuccess = false
			clustersWithMissingMetrics++
		}

		fmt.Printf("\nCluster: %s (%d/%d metrics present)\n", cluster, len(present), len(collectedMetrics))
		if len(missing) > 0 {
			fmt.Println("  Missing:")
			for _, m := range missing {
				fmt.Printf("    - %s\n", m)
			}
		}
	}

	if !allSuccess {
		fmt.Printf("\nSummary: %d clusters have missing metrics over %d.\n", clustersWithMissingMetrics, len(clusters))
		if len(missingAllClusters) > 0 {
			fmt.Println("The following metrics are missing in ALL clusters (should they be collected?):")
			for _, m := range missingAllClusters {
				fmt.Printf("  - %s\n", m)
			}
		}
		return fmt.Errorf("metrics are missing on some clusters")
	}

	greenCheckMark := "\033[32m" + "✓" + "\033[0m"
	fmt.Printf("\n%s All platform metrics are present across all managed clusters.\n", greenCheckMark)

	return nil
}

func fetchMetricAvailability(ctx context.Context, v1api v1.API, clusters []string, collectedMetrics []string, chunkSize int) (map[string]map[string]bool, error) {
	clusterMetricsPresent := make(map[string]map[string]bool)
	for _, c := range clusters {
		clusterMetricsPresent[c] = make(map[string]bool)
	}

	queryTime := time.Now()
	for i := 0; i < len(collectedMetrics); i += chunkSize {
		end := min(i+chunkSize, len(collectedMetrics))
		chunk := collectedMetrics[i:end]

		regexStr := fmt.Sprintf("^(%s)$", strings.Join(chunk, "|"))
		matcher, err := labels.NewMatcher(labels.MatchRegexp, "__name__", regexStr)
		if err != nil {
			return nil, fmt.Errorf("invalid metric regex: %w", err)
		}
		query := fmt.Sprintf("group by (cluster, __name__) ({%s})", matcher.String())
		result, warnings, err := v1api.Query(ctx, query, queryTime)
		if err != nil {
			return nil, fmt.Errorf("querying metrics chunk failed: %w", err)
		}
		if len(warnings) > 0 {
			fmt.Printf("Warnings: %v\n", warnings)
		}

		if result == nil {
			return nil, fmt.Errorf("querying metrics chunk failed: result is nil")
		}
		vec, ok := result.(model.Vector)
		if !ok {
			return nil, fmt.Errorf("expected Vector result, got %v", result.Type())
		}

		for _, sample := range vec {
			clusterName := string(sample.Metric["cluster"])
			metricName := string(sample.Metric["__name__"])
			if clusterName != "" {
				if _, ok := clusterMetricsPresent[clusterName]; ok {
					clusterMetricsPresent[clusterName][metricName] = true
				}
			}
		}
	}
	return clusterMetricsPresent, nil
}

func getManagedClusters(ctx context.Context, v1api v1.API, clusterRegex string) ([]string, error) {
	query := "group by (name) (acm_managed_cluster_labels)"
	if clusterRegex != "" {
		matcher, err := labels.NewMatcher(labels.MatchRegexp, "name", clusterRegex)
		if err != nil {
			return nil, fmt.Errorf("invalid cluster regex: %w", err)
		}
		query = fmt.Sprintf("group by (name) (acm_managed_cluster_labels{%s})", matcher.String())
	}
	result, warnings, err := v1api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings fetching clusters: %v\n", warnings)
	}

	if result == nil {
		return nil, fmt.Errorf("query failed: result is nil")
	}
	vec, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("expected Vector result for clusters, got %v", result.Type())
	}

	var clusters []string
	for _, sample := range vec {
		c := string(sample.Metric["name"])
		if c != "" {
			clusters = append(clusters, c)
		}
	}
	sort.Strings(clusters)
	return clusters, nil
}
