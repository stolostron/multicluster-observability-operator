DAHSBOARDS_DIR = ../../operators/multiclusterobservability/manifests/base/grafana/nexus
TMPDIR :=

.PHONY: check-platform-metrics
check-platform-metrics: create-tmpdir
	@$(CURDIR)/scripts/extract-dashboards-metrics.sh $(DAHSBOARDS_DIR)/acm | tr '\n' ',' > $(TMPDIR)/dash-metrics
	@go run cmd/dashcheck/main.go --scrape-configs=$(DAHSBOARDS_DIR)/acm/scrape-config.yaml \
		--dashboard-metrics=$$(cat $(TMPDIR)/dash-metrics) \
		--ignored-dashboard-metrics=cluster:memory_requested:ratio,cluster:memory_utilized:ratio,cluster:cpu_allocatable:sum,cluster:cpu_requested:ratio,cluster:cpu_cores:sum,acm_label_names,acm_managed_cluster_labels
	@cat $(DAHSBOARDS_DIR)/acm/prometheus-rule.yaml | yq '.spec' | promtool check rules
	@go run cmd/rulescheck/main.go --scrape-configs=$(DAHSBOARDS_DIR)/acm/scrape-config.yaml \
		--rules=$(DAHSBOARDS_DIR)/acm/prometheus-rule.yaml \
		--ignore-duplicated-rules=namespace_workload_pod:kube_pod_owner:relabel
	@rm -d $(TMPDIR)/dash-metrics $(TMPDIR)

.PHONY: check-rules
check-rules:
	cat ../../operators/multiclusterobservability/manifests/base/grafana/nexus/acm/prometheus-rule.yaml | yq '.spec' | promtool check rules

.PHONY: create-tmpdir
create-tmpdir:
	$(eval TMPDIR := $(shell mktemp -d))