/*
CI tool that provides a simple CLI to ensure that metrics resulting from rules evaluation defined in scrape configs
are defined in the listed rule files.
It ensures that rules are not duplicated and that no unneeded rule is defined.
*/
package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/stolostron/multicluster-observability-operator/cicd-scripts/metrics/internal/rule"
	"github.com/stolostron/multicluster-observability-operator/cicd-scripts/metrics/internal/scrapeconfig"
	"github.com/stolostron/multicluster-observability-operator/cicd-scripts/metrics/internal/utils"
)

func main() {
	scrapeConfigsArg := flag.String("scrape-configs", "", "Path to the comma separated scrape_configs")
	rulesArg := flag.String("rules", "", "Comma separated prometheus rules files")
	ignoreDupRulesArg := flag.String("ignore-duplicated-rules", "", "Comma separated ignored duplicated rules")
	greenCheckMark := "\033[32m" + "✓" + "\033[0m"
	flag.Parse()

	fmt.Println("Rulescheck — Verifying alignement of rules definition with the rules federated by the scrape configs.")

	if *scrapeConfigsArg == "" {
		fmt.Println("Please provide the scrape_configs paths")
		return
	}

	collectedRules, err := scrapeconfig.ReadFederatedMetrics(*scrapeConfigsArg)
	if err != nil {
		fmt.Printf("Failed to read scrape configs: %v", err)
		os.Exit(1)
	}

	collectedRules = slices.DeleteFunc(collectedRules, func(s string) bool { return !strings.Contains(s, ":") })
	if len(collectedRules) == 0 {
		fmt.Println(greenCheckMark, "No rule collected by the scrape configs.")
		return
	}

	if dups := utils.Duplicates(collectedRules); len(dups) > 0 {
		fmt.Println("Duplicate metrics found in scrape configs: ", dups)
		os.Exit(1)
	}

	if *rulesArg == "" {
		fmt.Println("Please provide prometheus rules files")
		return
	}

	promRulesList, err := rule.ReadFiles(*rulesArg)
	if err != nil {
		fmt.Println("Error reading prometheus rules: ", err)
		os.Exit(1)
	}

	if len(promRulesList) == 0 {
		fmt.Println("No prometheus rules found")
		os.Exit(1)
	}

	rulesDefined := []string{}
	for _, promRule := range promRulesList {
		if promRule == nil {
			fmt.Println("Rule is nil")
			os.Exit(1)
		}

		metrics, err := rule.RuleNames(promRule)
		if err != nil {
			fmt.Println("Error extracting metrics: ", err)
			os.Exit(1)
		}

		rulesDefined = append(rulesDefined, metrics...)
	}

	ignoredRules := strings.Split(*ignoreDupRulesArg, ",")
	rulesWithoutIgnoredDups := slices.DeleteFunc(slices.Clone(rulesDefined), func(s string) bool { return s == "" || slices.Contains(ignoredRules, s) })
	if dups := utils.Duplicates(rulesWithoutIgnoredDups); len(dups) > 0 {
		fmt.Println("Duplicate rules found in prometheus rules: ", dups)
		os.Exit(1)
	}

	added, removed := utils.Diff(collectedRules, rulesDefined)
	if len(added) > 0 {
		fmt.Println("Metrics found in scrape configs but not in rules: ", added)
		os.Exit(1)
	}

	if len(removed) > 0 {
		fmt.Println("Metrics found in rules but not in scrape configs: ", removed)
		os.Exit(1)
	}

	fmt.Println(greenCheckMark, "The rules collected by the scrapeConfigs are all defined, not more. Good job!")
}
