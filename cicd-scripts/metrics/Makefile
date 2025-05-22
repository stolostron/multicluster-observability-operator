# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

DAHSBOARDS_DIR = ../../operators/multiclusterobservability/manifests/base/grafana
PLATFORM_DASH_DIR = $(DAHSBOARDS_DIR)/platform-mcoa
HCP_DASH_DIR = $(DAHSBOARDS_DIR)/hcp-mcoa
ALERTS_DASH_DIR = $(DAHSBOARDS_DIR)/alerts
VIRTUALIZATION_DASH_DIR = $(DAHSBOARDS_DIR)/virtualization

TMPDIR := $(shell mktemp -d)

.PHONY: check-metrics check-platform-metrics check-hcp-metrics check-uwl-hcp-etcd-metrics check-uwl-hcp-apiserver-metrics check-alerts-metrics check-virtualization-metrics clean-tmpdir
check-metrics: check-platform-metrics check-hcp-metrics check-uwl-hcp-etcd-metrics check-uwl-hcp-apiserver-metrics check-alerts-metrics check-virtualization-metrics clean-tmpdir

check-platform-metrics: 
	@echo "--> Checking platform metrics:"
	@$(CURDIR)/scripts/extract-dashboards-metrics.sh -d $(PLATFORM_DASH_DIR) | tr '\n' ',' > $(TMPDIR)/dash-metrics
	@go run cmd/dashcheck/main.go --scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config.yaml \
		--dashboard-metrics=$$(cat $(TMPDIR)/dash-metrics)
	@cat $(PLATFORM_DASH_DIR)/prometheus-rule.yaml | yq '.spec' | promtool check rules
	@go run cmd/rulescheck/main.go --scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config.yaml \
		--rules=$(PLATFORM_DASH_DIR)/prometheus-rule.yaml \
		--ignored-rules-missing-definition=cluster:health:components:map \
		--ignore-duplicated-rules=namespace_workload_pod:kube_pod_owner:relabel
	@rm -d $(TMPDIR)/dash-metrics

check-hcp-metrics: 
	@echo "--> Checking hcp metrics:"
	@$(CURDIR)/scripts/extract-dashboards-metrics.sh -d $(HCP_DASH_DIR) | tr '\n' ',' > $(TMPDIR)/dash-metrics
	@go run cmd/dashcheck/main.go --scrape-configs=$(HCP_DASH_DIR)/scrape-config.yaml \
		--dashboard-metrics=$$(cat $(TMPDIR)/dash-metrics) \
		--ignored-dashboard-metrics=$(HUB_RULES) \
		--additional-scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config.yaml
	@go run cmd/rulescheck/main.go --scrape-configs=$(HCP_DASH_DIR)/scrape-config.yaml \
	@rm -d $(TMPDIR)/dash-metrics

check-uwl-hcp-etcd-metrics: 
	@echo "--> Checking uwl hcp metrics:"
	@$(CURDIR)/scripts/extract-dashboards-metrics.sh -f $(PLATFORM_DASH_DIR)/dash-k8s-etcd.yaml | tr '\n' ',' > $(TMPDIR)/dash-metrics
	@go run cmd/dashcheck/main.go --scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config-uwl-hcp-etcd.yaml \
		--dashboard-metrics=$$(cat $(TMPDIR)/dash-metrics) 
	@cat $(PLATFORM_DASH_DIR)/prometheus-rule-uwl-hcp-etcd.yaml | yq '.spec' | promtool check rules
	@go run cmd/rulescheck/main.go --scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config-uwl-hcp-etcd.yaml \
		--rules=$(PLATFORM_DASH_DIR)/prometheus-rule-uwl-hcp-etcd.yaml 
	@rm -d $(TMPDIR)/dash-metrics

check-uwl-hcp-apiserver-metrics: 
	@echo "--> Checking uwl apiserver metrics:"
	@$(CURDIR)/scripts/extract-dashboards-metrics.sh -f $(PLATFORM_DASH_DIR)/dash-k8s-apiserver.yaml | tr '\n' ',' > $(TMPDIR)/dash-metrics
	@go run cmd/dashcheck/main.go --scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config-uwl-hcp-apiserver.yaml \
		--ignored-scrapeconfig-metrics=apiserver_request_duration_seconds:histogram_quantile_99 \
		--dashboard-metrics=$$(cat $(TMPDIR)/dash-metrics) 
	@cat $(PLATFORM_DASH_DIR)/prometheus-rule-uwl-hcp-apiserver.yaml | yq '.spec' | promtool check rules
	@go run cmd/rulescheck/main.go --scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config-uwl-hcp-apiserver.yaml \
		--rules=$(PLATFORM_DASH_DIR)/prometheus-rule-uwl-hcp-apiserver.yaml 
	@rm -d $(TMPDIR)/dash-metrics

check-alerts-metrics: 
	@echo "--> Checking alert metrics:"
	@$(CURDIR)/scripts/extract-dashboards-metrics.sh -d $(ALERTS_DASH_DIR) | tr '\n' ',' > $(TMPDIR)/dash-metrics
	@go run cmd/dashcheck/main.go --scrape-configs=$(ALERTS_DASH_DIR)/scrape-config.yaml \
		--dashboard-metrics=$$(cat $(TMPDIR)/dash-metrics) \
		--ignored-dashboard-metrics=$(HUB_RULES) \
		--additional-scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config.yaml
	@rm -d $(TMPDIR)/dash-metrics

check-virtualization-metrics: 
	@echo "--> Checking virtualization metrics:"
	@$(CURDIR)/scripts/extract-dashboards-metrics.sh -d $(VIRTUALIZATION_DASH_DIR) | tr '\n' ',' > $(TMPDIR)/dash-metrics
	@go run cmd/dashcheck/main.go --scrape-configs=$(VIRTUALIZATION_DASH_DIR)/scrape-config.yaml \
		--dashboard-metrics=$$(cat $(TMPDIR)/dash-metrics) \
		--ignored-dashboard-metrics=$(HUB_RULES) \
		--ignored-scrapeconfig-metrics=kubevirt_vm_create_date_timestamp_seconds,kubevirt_vmi_memory_swap_in_traffic_bytes,kubevirt_vmi_vcpu_wait_seconds_total,kubevirt_vmi_memory_swap_out_traffic_bytes \
		--additional-scrape-configs=$(PLATFORM_DASH_DIR)/scrape-config.yaml,$(ALERTS_DASH_DIR)/scrape-config.yaml
	@rm -d $(TMPDIR)/dash-metrics

clean-tmpdir:
	@if [ -n "$(TMPDIR)" ] && [ -d "$(TMPDIR)" ]; then \
		rm -r "$(TMPDIR)"; \
		echo "Temporary directory $(TMPDIR) removed."; \
	fi
