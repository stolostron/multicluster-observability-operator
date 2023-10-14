# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

-include /opt/build-harness/Makefile.prow
include .bingo/Variables.mk

FILES_TO_FMT ?= $(shell find . -path ./vendor -prune -o -name '*.deepcopy.go' -prune -o -name '*.go' -print)
TMP_DIR := $(shell pwd)/tmp
BIN_DIR ?= $(TMP_DIR)/bin
GIT ?= $(shell which git)

# Image URL to use all building/pushing image targets
IMG ?= quay.io/stolostron/multicluster-observability-operator:latest

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: 
	cd operators/multiclusterobservability && make deploy

# UnDeploy controller from the configured Kubernetes cluster in ~/.kube/config
undeploy:
	cd operators/multiclusterobservability && make undeploy

# Build the docker image
docker-build:
	cd operators/multiclusterobservability && make manager
	docker build -t ${IMG} . -f operators/multiclusterobservability/Dockerfile	

# Push the docker image
docker-push:
	docker push ${IMG}

.PHONY: unit-tests
unit-tests: unit-tests-operators unit-tests-loaders unit-tests-proxy unit-tests-collectors

unit-tests-operators:
	go test -v ${VERBOSE} `go list ./operators/... | grep -v test`

unit-tests-loaders:
	go test -v ${VERBOSE} `go list ./loaders/... | grep -v test`

unit-tests-proxy:
	go test -v ${VERBOSE} `go list ./proxy/... | grep -v test`

unit-tests-collectors:
	go test ${VERBOSE} `go list ./collectors/... | grep -v test`

.PHONY: e2e-tests
e2e-tests:
	@echo "Running e2e tests ..."
	@./cicd-scripts/run-e2e-tests.sh

.PHONY: e2e-tests-in-kind
e2e-tests-in-kind:
	@echo "Running e2e tests in KinD cluster..."
ifeq ($(OPENSHIFT_CI),true)
	@./cicd-scripts/run-e2e-in-kind-via-prow.sh
else
	@./tests/run-in-kind/run-e2e-in-kind.sh
endif

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle:
	cd operators/multiclusterobservability && make bundle

.PHONY: check-git
check-git:
ifneq ($(GIT),)
	@test -x $(GIT) || (echo >&2 "No git executable binary found at $(GIT)."; exit 1)
else
	@echo >&2 "No git binary found."; exit 1
endif

.PHONY: deps
deps: ## Ensures fresh go.mod and go.sum.
	@go mod tidy
	@go mod verify
	@go mod vendor

.PHONY: go-format
go-format: ## Formats Go code including imports.
go-format: $(GOIMPORTS)
	@echo ">> formatting go code"
	@gofmt -s -w $(FILES_TO_FMT)
	@$(GOIMPORTS) -w $(FILES_TO_FMT)

.PHONY: shell-format
shell-format: $(SHFMT)
	@echo ">> formatting shell scripts"
	@$(SHFMT) -i 2 -ci -w -s $(shell find . -type f -name "*.sh" -not -path "*vendor*" -not -path "tmp/*")

.PHONY: format
format: ## Formats code including imports.
format: go-format shell-format

# PROTIP:
# Add
#      --cpu-profile-path string   Path to CPU profile output file
#      --mem-profile-path string   Path to memory profile output file
# to debug big allocations during linting.
.PHONY: go-lint
go-lint: check-git deps $(GOLANGCI_LINT) $(FAILLINT)
	@echo ">> verifying modules being imported"
	@$(FAILLINT) -paths "errors=github.com/efficientgo/core/errors,\
github.com/pkg/errors=github.com/efficientgo/core/errors,\
github.com/prometheus/tsdb=github.com/prometheus/prometheus/tsdb,\
github.com/prometheus/prometheus/pkg/testutils=github.com/thanos-io/thanos/pkg/testutil,\
github.com/prometheus/client_golang/prometheus.{DefaultGatherer,DefBuckets,NewUntypedFunc,UntypedFunc},\
github.com/prometheus/client_golang/prometheus.{NewCounter,NewCounterVec,NewCounterVec,NewGauge,NewGaugeVec,NewGaugeFunc,\
NewHistorgram,NewHistogramVec,NewSummary,NewSummaryVec}=github.com/prometheus/client_golang/prometheus/promauto.{NewCounter,\
NewCounterVec,NewCounterVec,NewGauge,NewGaugeVec,NewGaugeFunc,NewHistorgram,NewHistogramVec,NewSummary,NewSummaryVec},\
github.com/NYTimes/gziphandler.{GzipHandler}=github.com/klauspost/compress/gzhttp.{GzipHandler},\
sync/atomic=go.uber.org/atomic,\
io/ioutil.{Discard,NopCloser,ReadAll,ReadDir,ReadFile,TempDir,TempFile,Writefile}" $(shell go list ./... | grep -v "internal/cortex")
	@$(FAILLINT) -paths "fmt.{Print,Println}" -ignore-tests ./...
	@echo ">> examining all of the Go files"
	# @go vet -stdmethods=false ./...
	@echo ">> linting all of the Go files GOGC=${GOGC}"
	@$(GOLANGCI_LINT) run
	@echo ">> detecting misspells"
	@find . -type f | grep -v vendor/ | grep -vE '\./\..*' | gxargs $(MISSPELL) -error
	$(call require_clean_work_tree,'detected files without copyright, run make lint and commit changes')