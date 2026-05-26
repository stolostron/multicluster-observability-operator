# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

DAHSBOARDS_DIR = ../../operators/multiclusterobservability/manifests/base/grafana
PLATFORM_DASH_DIR = $(DAHSBOARDS_DIR)/platform-mcoa
HCP_DASH_DIR = $(DAHSBOARDS_DIR)/hcp-mcoa
ALERTS_DASH_DIR = $(DAHSBOARDS_DIR)/alerts
VIRTUALIZATION_DASH_DIR = $(DAHSBOARDS_DIR)/virtualization
HUB_RULES = cluster:memory_requested:ratio,cluster:memory_utilized:ratio,cluster:cpu_allocatable:sum,cluster:cpu_requested:ratio,cluster:cpu_cores:sum,acm_label_names,acm_managed_cluster_labels

TMPDIR := $(shell mktemp -d)

.PHONY: check-metrics check-platform-metrics check-hcp-metrics check-alerts-metrics check-virtualization-metrics clean-tmpdir
check-metrics: check-platform-metrics check-hcp-metrics check-alerts-metrics check-virtualization-metrics clean-tmpdir

check-platform-metrics: 
	@echo "--> Checking platform metrics:"
	@$(CURDIR)/scripts/extract-dashboards-metrics.sh $(PLATFORM_DASH_DIR) | tr '\n' ',' > $(TMPDIR)/dash-metrics
	@go run cmd/dashcheck/main.go --scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config.yaml \
		--dashboard-metrics=$$(cat $(TMPDIR)/dash-metrics) \
		--ignored-dashboard-metrics=$(HUB_RULES)
	@cat $(PLATFORM_DASH_DIR)/prometheus-rule.yaml | yq '.spec' | promtool check rules
	@go run cmd/rulescheck/main.go --scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config.yaml \
		--rules=$(PLATFORM_DASH_DIR)/prometheus-rule.yaml \
		--ignore-duplicated-rules=namespace_workload_pod:kube_pod_owner:relabel
	@rm -d $(TMPDIR)/dash-metrics

check-hcp-metrics: 
	@echo "--> Checking hcp metrics:"
	@$(CURDIR)/scripts/extract-dashboards-metrics.sh $(HCP_DASH_DIR) | tr '\n' ',' > $(TMPDIR)/dash-metrics
	@go run cmd/dashcheck/main.go --scrape-configs=$(HCP_DASH_DIR)/scrape-config.yaml \
		--dashboard-metrics=$$(cat $(TMPDIR)/dash-metrics) \
		--ignored-dashboard-metrics=$(HUB_RULES) \
		--additional-scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config.yaml
	@go run cmd/rulescheck/main.go --scrape-configs=$(HCP_DASH_DIR)/scrape-config.yaml \
	@rm -d $(TMPDIR)/dash-metrics

check-alerts-metrics: 
	@echo "--> Checking alert metrics:"
	@$(CURDIR)/scripts/extract-dashboards-metrics.sh $(ALERTS_DASH_DIR) | tr '\n' ',' > $(TMPDIR)/dash-metrics
	@go run cmd/dashcheck/main.go --scrape-configs=$(ALERTS_DASH_DIR)/scrape-config.yaml \
		--dashboard-metrics=$$(cat $(TMPDIR)/dash-metrics) \
		--ignored-dashboard-metrics=$(HUB_RULES) \
		--additional-scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config.yaml
	@rm -d $(TMPDIR)/dash-metrics

check-virtualization-metrics: 
	@echo "--> Checking virtualization metrics:"
	@$(CURDIR)/scripts/extract-dashboards-metrics.sh $(VIRTUALIZATION_DASH_DIR) | tr '\n' ',' > $(TMPDIR)/dash-metrics
	@go run cmd/dashcheck/main.go --scrape-configs=$(VIRTUALIZATION_DASH_DIR)/scrape-config.yaml \
		--dashboard-metrics=$$(cat $(TMPDIR)/dash-metrics) \
		--ignored-dashboard-metrics=$(HUB_RULES) \
		--additional-scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config.yaml,$(ALERTS_DASH_DIR)/scrape-config.yaml
	@rm -d $(TMPDIR)/dash-metrics

clean-tmpdir:
	@if [ -n "$(TMPDIR)" ] && [ -d "$(TMPDIR)" ]; then \
		rm -r "$(TMPDIR)"; \
		echo "Temporary directory $(TMPDIR) removed."; \
	fi
