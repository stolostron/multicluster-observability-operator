// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

/*
CI tool that provides a simple CLI to ensure that a list of metrics used in dashboards is well federated by
the referenced scrapeConfigs.
It ensures that metrics are not duplicated in scrape configs and that no unneeded metric is collected.
*/
package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/stolostron/multicluster-observability-operator/cicd-scripts/metrics/internal/scrapeconfig"
	"github.com/stolostron/multicluster-observability-operator/cicd-scripts/metrics/internal/utils"
)

func main() {
	scrapeConfigsArg := flag.String("scrape-configs", "", "Path to the comma separated scrape_configs")
	dashboardMetricsArg := flag.String("dashboard-metrics", "", "Comma separated dashboard metrics")
	ignoredDashboardMetricsArg := flag.String("ignored-dashboard-metrics", "", "Comma separated ignored dashboard metrics. For example, rules that are computed on the hub instead of being collected from the spokes.")
	additionalScrapeConfigsArg := flag.String("additional-scrape-configs", "", "Path to the comma separated scrape_configs that are collected in addition of the main one. Over collected metrics from them are ignored.")
	flag.Parse()

	fmt.Println("Dashcheck — Verifying alignement of federated metrics from scrape configs for dashboard metrics.")

	if *scrapeConfigsArg == "" {
		fmt.Println("Please provide the scrape_configs paths")
		return
	}

	if *dashboardMetricsArg == "" {
		fmt.Println("Please provide the dashboard metrics")
		return
	}

	ignoredDashboardMetrics := strings.Split(*ignoredDashboardMetricsArg, ",")

	dashboardMetrics := strings.Split(*dashboardMetricsArg, ",")
	dashboardMetrics = slices.DeleteFunc(dashboardMetrics, func(s string) bool { return s == "" || slices.Contains(ignoredDashboardMetrics, s) })
	if len(dashboardMetrics) == 0 {
		fmt.Println("No dashboard metrics found")
		os.Exit(1)
	}

	collectedMetrics, err := scrapeconfig.ReadFederatedMetrics(*scrapeConfigsArg)
	if err != nil {
		fmt.Printf("Failed to read scrape configs: %v", err)
		os.Exit(1)
	}

	var additionalMetrics []string
	if len(*additionalScrapeConfigsArg) > 0 {
		additionalMetrics, err = scrapeconfig.ReadFederatedMetrics(*additionalScrapeConfigsArg)
		if err != nil {
			fmt.Printf("Failed to read additional scrape configs: %v", err)
			os.Exit(1)
		}

		collectedMetrics = append(collectedMetrics, additionalMetrics...)
	}

	if dups := utils.Duplicates(collectedMetrics); len(dups) > 0 {
		fmt.Println("Duplicate metrics found in scrape configs: ", dups)
		os.Exit(1)
	}

	added, removed := utils.Diff(dashboardMetrics, collectedMetrics)
	// Remove additional metrics from the added list
	// They must be ignored
	added = slices.DeleteFunc(added, func(s string) bool { return s == "" || slices.Contains(additionalMetrics, s) })
	if len(added) > 0 {
		fmt.Println("Metrics found in scrape configs but not in dashboards: ", added)
		os.Exit(1)
	}

	if len(removed) > 0 {
		fmt.Println("Metrics found in dashboards but not in scrape configs: ", removed)
		os.Exit(1)
	}

	greenCheckMark := "\033[32m" + "✓" + "\033[0m"
	fmt.Println(greenCheckMark, "Scrape configs are collecting all dashboards metrics, not more. Good job!")
}
