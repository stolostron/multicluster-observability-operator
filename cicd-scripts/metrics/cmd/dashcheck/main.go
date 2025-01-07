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
	ignoredDashboardMetricsArg := flag.String("ignored-dashboard-metrics", "", "Comma separated ignored dashboard metrics")
	flag.Parse()

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

	scrapeConfigsList, err := scrapeconfig.ReadFiles(*scrapeConfigsArg)
	if err != nil {
		fmt.Println("Error reading scrape configs: ", err)
		os.Exit(1)
	}

	if len(scrapeConfigsList) == 0 {
		fmt.Println("No scrape configs found")
		os.Exit(1)
	}

	collectedMetrics := []string{}
	for _, scrapeConfig := range scrapeConfigsList {
		if scrapeConfig == nil {
			fmt.Println("Scrape config is nil")
			os.Exit(1)
		}

		metrics, err := scrapeconfig.FederatedMetrics(scrapeConfig)
		if err != nil {
			fmt.Println("Error extracting metrics: ", err)
			os.Exit(1)
		}

		if dups := utils.Duplicates(metrics); len(dups) > 0 {
			fmt.Printf("Duplicate metrics found in %s: %v", scrapeConfig.Name, dups)
			os.Exit(1)
		}

		collectedMetrics = append(collectedMetrics, metrics...)
	}

	if dups := utils.Duplicates(collectedMetrics); len(dups) > 0 {
		fmt.Println("Duplicate metrics found in scrape configs: ", dups)
		os.Exit(1)
	}

	added, removed := utils.Diff(dashboardMetrics, collectedMetrics)
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

// func readScrapeConfigs(scrapeConfigsPath string) ([]*prometheusalpha1.ScrapeConfig, error) {
// 	paths := strings.Split(scrapeConfigsPath, ",")
// 	ret := []*prometheusalpha1.ScrapeConfig{}
// 	for _, path := range paths {
// 		fmt.Println("Reading scrape config: ", path)
// 		res, err := scrapeconfig.ReadFile(path)
// 		if err != nil {
// 			return nil, err
// 		}
// 		ret = append(ret, res)
// 	}

// 	return ret, nil
// }

// func readScrapeConfig(scrapeConfigsPath string) (*prometheusalpha1.ScrapeConfig, error) {
// 	fileData, err := os.ReadFile(scrapeConfigsPath)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to read file %s: %w", scrapeConfigsPath, err)
// 	}

// 	scrapeConfig := &prometheusalpha1.ScrapeConfig{}
// 	if err := yaml.Unmarshal(fileData, scrapeConfig); err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal file %s: %w", scrapeConfigsPath, err)
// 	}

// 	return scrapeConfig, nil
// }

// func extractCollectedMetrics(scrapeConfig *prometheusalpha1.ScrapeConfig) ([]string, error) {
// 	ret := []string{}
// 	for _, query := range scrapeConfig.Spec.Params["match[]"] {
// 		expr, err := parser.ParseExpr(query)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to parse query %s: %w", query, err)
// 		}

// 		switch v := expr.(type) {
// 		case *parser.VectorSelector:
// 			for _, matcher := range v.LabelMatchers {
// 				if matcher.Name == "__name__" {
// 					ret = append(ret, matcher.Value)
// 				}
// 			}
// 		default:
// 			return nil, fmt.Errorf("unsupported expression type: %T", v)
// 		}
// 	}

// 	return ret, nil
// }

// func getDuplicates(elements []string) []string {
// 	found := map[string]struct{}{}
// 	ret := []string{}
// 	for _, element := range elements {
// 		if _, ok := found[element]; ok {
// 			ret = append(ret, element)
// 		} else {
// 			found[element] = struct{}{}
// 		}
// 	}
// 	return ret
// }

// func diff(a, b []string) (added, removed []string) {
// 	mA := make(map[string]struct{}, len(a))
// 	for _, x := range a {
// 		mA[x] = struct{}{}
// 	}

// 	mB := make(map[string]struct{}, len(b))
// 	for _, x := range b {
// 		mB[x] = struct{}{}
// 	}

// 	// Identify elements in b that are not in a
// 	for x := range mB {
// 		if _, ok := mA[x]; !ok {
// 			added = append(added, x)
// 		}
// 	}

// 	// Identify elements in a that are not in b
// 	for x := range mA {
// 		if _, ok := mB[x]; !ok {
// 			removed = append(removed, x)
// 		}
// 	}

// 	return added, removed
// }
