# Copyright Contributors to the Open Cluster Management project

METRICS_IMAGE?=quay.io/haoqing/metrics-data:latest
METRICS_JSON=./_output/metrics.json
BIN_DIR?=$(shell pwd)/_output/bin
GOJSONTOYAML_BIN=$(BIN_DIR)/gojsontoyaml

export PATH := $(BIN_DIR):$(PATH)

all: timeseries build push

timeseries: $(METRICS_JSON)
	oc port-forward -n openshift-monitoring prometheus-k8s-0 9090 > /dev/null & \
	sleep 50 ; \
	query="curl --fail --silent -G http://localhost:9090/federate"; \
	for rule in $$(cat $(METRICS_JSON) | jq -r '.[]'); do \
	    query="$$query $$(printf -- "--data-urlencode match[]=%s" $$rule)"; \
	done; \
	echo '# This file was generated using `make $@`.' > $@.txt ; \
	$$query >> $@.txt ; \
	jobs -p | xargs -r kill

$(METRICS_JSON): $(GOJSONTOYAML_BIN)
	matches=`curl -L https://raw.githubusercontent.com/open-cluster-management/multicluster-observability-operator/main/manifests/base/config/metrics_allowlist.yaml | \
	    $(GOJSONTOYAML_BIN) --yamltojson | jq -r '.data."metrics_list.yaml"' | $(GOJSONTOYAML_BIN) --yamltojson | jq -r '.matches' | jq '"{" + .[] + "}"'`; \
	names=`curl -L https://raw.githubusercontent.com/open-cluster-management/multicluster-observability-operator/main/manifests/base/config/metrics_allowlist.yaml | \
	    $(GOJSONTOYAML_BIN) --yamltojson | jq -r '.data."metrics_list.yaml"' | $(GOJSONTOYAML_BIN) --yamltojson | jq -r '.names' | jq '"{__name__=\"" + .[] + "\"}"'`; \
	echo $$matches $$names | jq -s . > $@

$(GOJSONTOYAML_BIN): $(BIN_DIR)
	GOBIN=$(BIN_DIR) go get github.com/brancz/gojsontoyaml

$(BIN_DIR):
	mkdir -p $@

build:
	docker build -t $(METRICS_IMAGE) .

push:
	docker push $(METRICS_IMAGE)

clean:
	rm -r _output && rm timeseries.txt
