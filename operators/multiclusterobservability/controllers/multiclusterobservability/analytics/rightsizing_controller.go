package analytics

import (
	"github.com/cloudflare/cfssl/log"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	rsPolicySetName                   = "rs-policyset"
	rsPlacementName                   = "rs-placement"
	rsPlacementBindingName            = "rs-policyset-binding"
	rsPrometheusRulePolicyName        = "rs-prom-rules-policy"
	rsPrometheusRulePolicyConfigName  = "rs-prometheus-rules-policy-config"
	rsPrometheusRuleName              = "rs-namespace-prometheus-rules"
	rsConfigMapName                   = "rs-namespace-config"
	rsDefaultNamespace                = "open-cluster-management-global-set"
	rsMonitoringNamespace             = "openshift-monitoring"
	rsDefaultRecommendationPercentage = 110
)

var (
	rsNamespace = rsDefaultNamespace
)

type RSLabelFilter struct {
	LabelName         string   `yaml:"labelName"`
	InclusionCriteria []string `yaml:"inclusionCriteria,omitempty"`
	ExclusionCriteria []string `yaml:"exclusionCriteria,omitempty"`
}

type RSPrometheusRuleConfig struct {
	NamespaceFilterCriteria struct {
		InclusionCriteria []string `yaml:"inclusionCriteria"`
		ExclusionCriteria []string `yaml:"exclusionCriteria"`
	} `yaml:"namespaceFilterCriteria"`
	LabelFilterCriteria      []RSLabelFilter `yaml:"labelFilterCriteria"`
	RecommendationPercentage int             `yaml:"recommendationPercentage"`
}

type RSNamespaceConfigMapData struct {
	PrometheusRuleConfig   RSPrometheusRuleConfig   `yaml:"prometheusRuleConfig"`
	PlacementConfiguration clusterv1beta1.Placement `yaml:"placementConfiguration"`
}

// CreateRightSizingComponent is used to enable the right sizing recommendation
func CreateRightSizingComponent(
	c client.Client,
	scheme *runtime.Scheme,
	mco *mcov1beta2.MultiClusterObservability,
	mgr ctrl.Manager) (*ctrl.Result, error) {

	log.Info("inside CreateRightSizingComponent")

	// Check if the analytics right-sizing namespace recommendation configuration is enabled
	if isRightSizingNamespaceEnabled(mco) {

		if mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.NamespaceBinding != "" {
			rsNamespace = mco.Spec.Capabilities.Platform.Analytics.NamespaceRightSizingRecommendation.NamespaceBinding
		}

		// Call the function to ensure the ConfigMap exists
		err := ensureRSNamespaceConfigMapExists(c)
		if err != nil {
			return &ctrl.Result{}, err
		}

		log.Info("RS - Analytics.NamespaceRightSizing resource creation completed")
	} else {

		// Change ComplianceType to "MustNotHave" for PrometheusRule deletion
		// As deleting Policy doesn't explicitly delete related PrometheusRule
		modifyComplianceTypeIfPolicyExists(c)

		// Cleanup created resources if available
		cleanupRSNamespaceResources(c, rsNamespace)
	}

	log.Info("RS - CreateRightSizingComponent task completed6")
	return nil, nil
}
