package scrapeconfig

import (
	"fmt"
	"os"
	"strings"

	prometheusalpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/prometheus/prometheus/promql/parser"
	"sigs.k8s.io/yaml"
)

func ReadFile(scrapeConfigsPath string) (*prometheusalpha1.ScrapeConfig, error) {
	fileData, err := os.ReadFile(scrapeConfigsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", scrapeConfigsPath, err)
	}

	scrapeConfig := &prometheusalpha1.ScrapeConfig{}
	if err := yaml.Unmarshal(fileData, scrapeConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal file %s: %w", scrapeConfigsPath, err)
	}

	return scrapeConfig, nil
}

func ReadFiles(scrapeConfigsPath string) ([]*prometheusalpha1.ScrapeConfig, error) {
	paths := strings.Split(scrapeConfigsPath, ",")
	ret := []*prometheusalpha1.ScrapeConfig{}
	for _, path := range paths {
		fmt.Println("Reading scrape config: ", path)
		res, err := ReadFile(path)
		if err != nil {
			return nil, err
		}
		ret = append(ret, res)
	}

	return ret, nil
}

// FederatedMetrics returns the list of collected metrics from a scrapeConfig, parsing the
// federated metrics list.
func FederatedMetrics(scrapeConfig *prometheusalpha1.ScrapeConfig) ([]string, error) {
	ret := []string{}
	for _, query := range scrapeConfig.Spec.Params["match[]"] {
		expr, err := parser.ParseExpr(query)
		if err != nil {
			return nil, fmt.Errorf("failed to parse query %s: %w", query, err)
		}

		switch v := expr.(type) {
		case *parser.VectorSelector:
			for _, matcher := range v.LabelMatchers {
				if matcher.Name == "__name__" {
					ret = append(ret, matcher.Value)
				}
			}
		default:
			return nil, fmt.Errorf("unsupported expression type: %T", v)
		}
	}

	return ret, nil
}
