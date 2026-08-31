// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package mcoa

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	prometheusv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stolostron/multicluster-observability-operator/operators/endpointmetrics/pkg/hypershift"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	hypershiftReconcileTriggerName = "hypershift-reconcile"

	etcdHcpComponentLabel      = "etcd-hcp-user-workload-metrics-collector"
	apiserverHcpComponentLabel = "apiserver-hcp-user-workload-metrics-collector"

	clusterIDMetricLabel             = "clusterID"
	clusterNameMetricLabel           = "cluster"
	managementClusterIDMetricLabel   = "managementclusterID"
	managementClusterNameMetricLabel = "managementcluster"
)

type hcpClusterIdentity struct {
	id   string
	name string
}

// ReconcileHyperShift creates ACM ServiceMonitors for Hypershift hosted clusters so
// UWL Prometheus can scrape HCP etcd and apiserver metrics for federation by PrometheusAgent.
func (r *MCOAAgentReconciler) ReconcileHyperShift(ctx context.Context) error {
	isHypershift, err := hypershift.IsHypershiftCluster()
	if err != nil {
		return fmt.Errorf("failed to check if the cluster is hypershift: %w", err)
	}
	if !isHypershift {
		r.Log.Info("Hypershift CRD not present, skipping hosted cluster ServiceMonitor reconciliation")
		return nil
	}

	if r.Namespace == "" {
		return fmt.Errorf("namespace is required for MCOA hypershift reconcile")
	}

	etcdScrapeConfigs, err := r.listScrapeConfigsByComponent(ctx, etcdHcpComponentLabel)
	if err != nil {
		return fmt.Errorf("failed to list etcd HCP scrape configs: %w", err)
	}
	if len(etcdScrapeConfigs) == 0 {
		r.Log.V(1).Info("no etcd HCP scrape configs found", "namespace", r.Namespace, "component", etcdHcpComponentLabel)
	}

	apiserverScrapeConfigs, err := r.listScrapeConfigsByComponent(ctx, apiserverHcpComponentLabel)
	if err != nil {
		return fmt.Errorf("failed to list apiserver HCP scrape configs: %w", err)
	}
	if len(apiserverScrapeConfigs) == 0 {
		r.Log.V(1).Info("no apiserver HCP scrape configs found", "namespace", r.Namespace, "component", apiserverHcpComponentLabel)
	}

	etcdRules, err := r.listPrometheusRulesByComponent(ctx, etcdHcpComponentLabel)
	if err != nil {
		return fmt.Errorf("failed to list etcd HCP prometheus rules: %w", err)
	}

	apiserverRules, err := r.listPrometheusRulesByComponent(ctx, apiserverHcpComponentLabel)
	if err != nil {
		return fmt.Errorf("failed to list apiserver HCP prometheus rules: %w", err)
	}

	etcdMetrics, err := r.extractDependentMetrics(etcdScrapeConfigs, etcdRules)
	if err != nil {
		return fmt.Errorf("failed to extract etcd dependent metrics: %w", err)
	}

	apiserverMetrics, err := r.extractDependentMetrics(apiserverScrapeConfigs, apiserverRules)
	if err != nil {
		return fmt.Errorf("failed to extract apiserver dependent metrics: %w", err)
	}

	hostedClusters := &hyperv1.HostedClusterList{}
	if err := r.List(ctx, hostedClusters, &client.ListOptions{}); err != nil {
		return fmt.Errorf("failed to list HostedClusterList: %w", err)
	}

	for _, hostedCluster := range hostedClusters.Items {
		if len(hostedCluster.Spec.ClusterID) == 0 {
			r.Log.Info("hosted cluster is missing clusterID, skipping ServiceMonitor creation", "name", hostedCluster.Name)
			continue
		}

		namespace := hypershift.HostedClusterNamespace(&hostedCluster)
		identity := hcpClusterIdentity{
			id:   hostedCluster.Spec.ClusterID,
			name: hostedCluster.Name,
		}

		etcdSMDesired, err := r.generateMCOAEtcdServiceMonitor(ctx, namespace, identity, etcdMetrics)
		if err != nil {
			r.Log.Error(err, "failed to generate etcd ServiceMonitor", "namespace", namespace)
			continue
		}
		if etcdSMDesired != nil {
			if err := hypershift.CreateOrUpdateServiceMonitor(ctx, r.Client, etcdSMDesired); err != nil {
				r.Log.Error(err, "failed to create/update etcd ServiceMonitor", "namespace", namespace)
				continue
			}
		}

		apiServerSMDesired, err := r.generateMCOAApiServerServiceMonitor(ctx, namespace, identity, apiserverMetrics)
		if err != nil {
			r.Log.Error(err, "failed to generate api server ServiceMonitor", "namespace", namespace)
			continue
		}
		if apiServerSMDesired != nil {
			if err := hypershift.CreateOrUpdateServiceMonitor(ctx, r.Client, apiServerSMDesired); err != nil {
				r.Log.Error(err, "failed to create/update api-server ServiceMonitor", "namespace", namespace)
				continue
			}
		}
	}

	return nil
}

func isHypershiftReconcileRequest(req ctrl.Request) bool {
	return req.Name == hypershiftReconcileTriggerName && req.Namespace == ""
}

func (r *MCOAAgentReconciler) listPrometheusRulesByComponent(ctx context.Context, component string) ([]promv1.PrometheusRule, error) {
	list := &promv1.PrometheusRuleList{}
	if err := r.List(ctx, list,
		client.InNamespace(r.Namespace),
		client.MatchingLabels{labelKeyComponent: component},
	); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *MCOAAgentReconciler) extractDependentMetrics(scrapeConfigs []prometheusv1alpha1.ScrapeConfig, rules []promv1.PrometheusRule) ([]string, error) {
	scPtrs := make([]*prometheusv1alpha1.ScrapeConfig, len(scrapeConfigs))
	for i := range scrapeConfigs {
		scPtrs[i] = &scrapeConfigs[i]
	}

	scMetrics, err := r.federatedMetrics(scPtrs)
	if err != nil {
		return nil, fmt.Errorf("failed to extract federated metrics from ScrapeConfig: %w", err)
	}

	dedup := make(map[string]struct{}, len(scMetrics))
	for _, metricsName := range scMetrics {
		dedup[metricsName] = struct{}{}
	}

	rulePtrs := make([]*promv1.PrometheusRule, len(rules))
	for i := range rules {
		rulePtrs[i] = &rules[i]
	}

	ruleMetrics, err := r.rulesDependentMetrics(rulePtrs)
	if err != nil {
		return nil, fmt.Errorf("failed to extract dependent metrics from PrometheusRule: %w", err)
	}

	for _, metricsName := range ruleMetrics {
		dedup[metricsName] = struct{}{}
	}

	return slices.Sorted(maps.Keys(dedup)), nil
}

func (r *MCOAAgentReconciler) federatedMetrics(scrapeConfigs []*prometheusv1alpha1.ScrapeConfig) ([]string, error) {
	ret := []string{}

	for _, scrapeConfig := range scrapeConfigs {
		if scrapeConfig == nil {
			continue
		}

		for _, query := range scrapeConfig.Spec.Params["match[]"] {
			expr, err := parser.ParseExpr(query)
			if err != nil {
				return nil, fmt.Errorf("failed to parse query %s: %w", query, err)
			}

			selectors := parser.ExtractSelectors(expr)
			for _, node := range selectors {
				for _, matcher := range node {
					if matcher.Name != "__name__" || isRuleMetricName(matcher.Value) {
						continue
					}

					if matcher.Type != labels.MatchEqual {
						r.Log.V(1).Info("ignoring non equal type labels matcher in scrapeConfig, not supported", "scrapeConfig", scrapeConfig.Name, "matcher", matcher.String())
						continue
					}

					ret = append(ret, matcher.Value)
				}
			}
		}
	}

	return ret, nil
}

func (r *MCOAAgentReconciler) rulesDependentMetrics(promRules []*promv1.PrometheusRule) ([]string, error) {
	ret := []string{}

	for _, promRule := range promRules {
		if promRule == nil {
			continue
		}

		for _, group := range promRule.Spec.Groups {
			for _, rule := range group.Rules {
				expr, err := parser.ParseExpr(rule.Expr.StrVal)
				if err != nil {
					return nil, fmt.Errorf("failed to parse query for rule named %q: %w", rule.Record, err)
				}

				selectors := parser.ExtractSelectors(expr)
				for _, node := range selectors {
					for _, matcher := range node {
						if matcher.Name != "__name__" || isRuleMetricName(matcher.Value) {
							continue
						}

						if matcher.Type != labels.MatchEqual {
							r.Log.V(1).Info("ignoring non equal type labels matcher in rule, not supported", "promRule", promRule.Name, "matcher", matcher.String())
							continue
						}

						ret = append(ret, matcher.Value)
					}
				}
			}
		}
	}

	return ret, nil
}

func (r *MCOAAgentReconciler) generateMCOAEtcdServiceMonitor(
	ctx context.Context,
	namespace string,
	hostedCluster hcpClusterIdentity,
	metrics []string,
) (*promv1.ServiceMonitor, error) {
	if len(metrics) == 0 {
		r.Log.V(1).Info("no metrics to collect for etcd, skipping serviceMonitor creation", "hostedClusterName", hostedCluster.name)
		return nil, nil
	}

	hypershiftEtcdSM := &promv1.ServiceMonitor{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: hypershift.EtcdSmName}, hypershiftEtcdSM); err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Error(err, "hypershift etcd ServiceMonitor not found, cannot set observability for etcd", "namespace", namespace, "hostedClusterName", hostedCluster.name)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get hypershift's etcd ServiceMonitor: %w", err)
	}

	ret := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hypershift.AcmEtcdSmName,
			Namespace: namespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			Selector:          hypershiftEtcdSM.Spec.Selector,
			NamespaceSelector: hypershiftEtcdSM.Spec.NamespaceSelector,
		},
	}

	for _, endpoint := range hypershiftEtcdSM.Spec.Endpoints {
		ret.Spec.Endpoints = append(ret.Spec.Endpoints, promv1.Endpoint{
			Interval:             "30s",
			Scheme:               endpoint.Scheme,
			Port:                 endpoint.Port,
			TargetPort:           endpoint.TargetPort,
			TLSConfig:            endpoint.TLSConfig,
			MetricRelabelConfigs: r.generateMCOAMetricsRelabelConfigs(hostedCluster, metrics),
			RelabelConfigs: []promv1.RelabelConfig{
				{
					TargetLabel: "job",
					Action:      "replace",
					Replacement: ptr.To("etcd"),
				},
			},
		})
	}

	return ret, nil
}

func (r *MCOAAgentReconciler) generateMCOAApiServerServiceMonitor(
	ctx context.Context,
	namespace string,
	hostedCluster hcpClusterIdentity,
	metrics []string,
) (*promv1.ServiceMonitor, error) {
	if len(metrics) == 0 {
		r.Log.V(1).Info("no metrics to collect for apiserver, skipping serviceMonitor creation", "hostedClusterName", hostedCluster.name)
		return nil, nil
	}

	hypershiftApiServerSM := &promv1.ServiceMonitor{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: hypershift.ApiServerSmName}, hypershiftApiServerSM); err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Error(err, "hypershift apiserver ServiceMonitor not found, cannot set observability for api server", "namespace", namespace, "hostedClusterName", hostedCluster.name)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get hypershift's kube-apiserver ServiceMonitor: %w", err)
	}

	ret := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hypershift.AcmApiServerSmName,
			Namespace: namespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			Selector:          hypershiftApiServerSM.Spec.Selector,
			NamespaceSelector: hypershiftApiServerSM.Spec.NamespaceSelector,
		},
	}

	for _, endpoint := range hypershiftApiServerSM.Spec.Endpoints {
		ret.Spec.Endpoints = append(ret.Spec.Endpoints, promv1.Endpoint{
			Interval:             "30s",
			Scheme:               endpoint.Scheme,
			Port:                 endpoint.Port,
			TargetPort:           endpoint.TargetPort,
			TLSConfig:            endpoint.TLSConfig,
			MetricRelabelConfigs: r.generateMCOAMetricsRelabelConfigs(hostedCluster, metrics),
			RelabelConfigs: []promv1.RelabelConfig{
				{
					TargetLabel: "job",
					Action:      "replace",
					Replacement: ptr.To("apiserver"),
				},
			},
		})
	}

	return ret, nil
}

func (r *MCOAAgentReconciler) generateMCOAMetricsRelabelConfigs(hostedCluster hcpClusterIdentity, metrics []string) []promv1.RelabelConfig {
	return []promv1.RelabelConfig{
		{
			SourceLabels: []promv1.LabelName{"__name__"},
			Action:       "keep",
			Regex:        fmt.Sprintf("(%s)", strings.Join(metrics, "|")),
		},
		{
			TargetLabel: clusterIDMetricLabel,
			Action:      "replace",
			Replacement: &hostedCluster.id,
		},
		{
			TargetLabel: clusterNameMetricLabel,
			Action:      "replace",
			Replacement: &hostedCluster.name,
		},
		{
			TargetLabel: managementClusterIDMetricLabel,
			Action:      "replace",
			Replacement: &r.ClusterID,
		},
		{
			TargetLabel: managementClusterNameMetricLabel,
			Action:      "replace",
			Replacement: &r.ClusterName,
		},
	}
}

func isRuleMetricName(name string) bool {
	return strings.Contains(name, ":")
}
