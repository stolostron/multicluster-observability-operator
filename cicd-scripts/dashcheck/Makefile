.PHONY: check-metrics
check-metrics:
	./extract-dashboards-metrics.sh ../../operators/multiclusterobservability/manifests/base/grafana/nexus/acm | tr '\n' ',' > /tmp/dash-metrics
	go run main.go --scrape-configs=../../operators/multiclusterobservability/manifests/base/grafana/nexus/acm/scrape-config.yaml --dashboard-metrics=$$(cat /tmp/dash-metrics)
	rm /tmp/dash-metrics

.PHONY: check-rules
check-rules:
	cat ../../operators/multiclusterobservability/manifests/base/grafana/nexus/acm/prometheus-rule.yaml | yq '.spec' | promtool check rules