# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

-include /opt/build-harness/Makefile.prow
include .bingo/Variables.mk

FILES_TO_FMT ?= $(shell find . -path ./vendor -prune -o -name '*.deepcopy.go' -prune -o -name '*.go' -print)
TMP_DIR := $(shell pwd)/tmp
BIN_DIR ?= $(TMP_DIR)/bin
export PATH := $(BIN_DIR):$(PATH)
GIT ?= $(shell which git)

# Support gsed on OSX (installed via brew), falling back to sed. On Linux
# systems gsed won't be installed, so will use sed as expected.
export SED ?= $(shell which gsed 2>/dev/null || which sed)

XARGS ?= $(shell which gxargs 2>/dev/null || which xargs)
GREP ?= $(shell which ggrep 2>/dev/null || which grep)

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ General

# The help target prints out all targtoolsets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php
.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

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
shell-format: $(SHFMT) ## Formats shell code.
	@echo ">> formatting shell scripts"
	@$(SHFMT) -i 2 -ci -w -s $(shell find . -type f -name "*.sh" -not -path "*vendor*" -not -path "tmp/*")

.PHONY: format
format: ## Formats code including imports.
format: go-format shell-format

define require_clean_work_tree
	@git update-index -q --ignore-submodules --refresh

	@if ! git diff-files --quiet --ignore-submodules --; then \
		echo >&2 "cannot $1: you have unstaged changes."; \
		git diff -r --ignore-submodules -- >&2; \
		echo >&2 "Please commit or stash them."; \
		exit 1; \
	fi

	@if ! git diff-index --cached --quiet HEAD --ignore-submodules --; then \
		echo >&2 "cannot $1: your index contains uncommitted changes."; \
		git diff --cached -r --ignore-submodules HEAD -- >&2; \
		echo >&2 "Please commit or stash them."; \
		exit 1; \
	fi

endef

.PHONY: verify-containerfile-labels
verify-containerfile-labels: ## Verify Containerfile.operator RHEL version consistency
	@echo ">> verifying Containerfile.operator label consistency"
	@./scripts/verify-containerfile-labels.sh

# PROTIP:
# Add
#      --cpu-profile-path string   Path to CPU profile output file
#      --mem-profile-path string   Path to memory profile output file
# to debug big allocations during linting.
.PHONY: lint
lint: check-git deps format verify-containerfile-labels $(GOLANGCI_LINT) $(FAILLINT)
	$(call require_clean_work_tree,'detected files without copyright, run make lint and commit changes')
	@echo ">> verifying modules being imported"
	@$(FAILLINT) -paths "github.com/prometheus/tsdb=github.com/prometheus/prometheus/tsdb,\
github.com/prometheus/prometheus/pkg/testutils=github.com/thanos-io/thanos/pkg/testutil,\
github.com/prometheus/client_golang/prometheus.{DefaultGatherer,DefBuckets,NewUntypedFunc,UntypedFunc},\
github.com/prometheus/client_golang/prometheus.{NewCounter,NewCounterVec,NewCounterVec,NewGauge,NewGaugeVec,NewGaugeFunc,\
NewHistorgram,NewHistogramVec,NewSummary,NewSummaryVec}=github.com/prometheus/client_golang/prometheus/promauto.{NewCounter,\
NewCounterVec,NewCounterVec,NewGauge,NewGaugeVec,NewGaugeFunc,NewHistorgram,NewHistogramVec,NewSummary,NewSummaryVec},\
github.com/NYTimes/gziphandler.{GzipHandler}=github.com/klauspost/compress/gzhttp.{GzipHandler},\
sync/atomic=go.uber.org/atomic,\
io/ioutil.{Discard,NopCloser,ReadAll,ReadDir,ReadFile,TempDir,TempFile,Writefile}" ./operators/... ./collectors/... ./loaders/... ./proxy/...
	@$(FAILLINT) -paths "fmt.{Print,Println}" -ignore-tests ./operators/... ./collectors/... ./loaders/... ./proxy/...
	@echo ">> examining all of the Go files"
	@go vet -stdmethods=false ./...
	@echo ">> linting all of the Go files GOGC=${GOGC}"
	@$(GOLANGCI_LINT) run
	@echo ">> ensuring Copyright headers"
	@go run ./scripts/copyright
	$(call require_clean_work_tree,'detected files without copyright, run make lint and commit changes')

.PHONY: check-metrics
check-metrics:
	@$(MAKE) -C cicd-scripts/metrics check-metrics

.PHONY: unit-tests ## Run all unit tests.
unit-tests: unit-tests-operators unit-tests-loaders unit-tests-proxy unit-tests-collectors

.PHONY: unit-tests-operators
unit-tests-operators:  ## Run operators unit tests only.
	go test ${VERBOSE} `go list ./operators/... | $(GREP) -v test`

.PHONY: unit-tests-loaders
unit-tests-loaders: ## Run loaders unit tests only.
	go test ${VERBOSE} `go list ./loaders/... | $(GREP) -v test`

.PHONY: unit-tests-proxy
unit-tests-proxy: ## Run proxy uni tests only.
	go test ${VERBOSE} `go list ./proxy/... | $(GREP) -v test`

.PHONY: unit-tests-collectors
unit-tests-collectors: ## Run collectors unit tests only. 
	go test ${VERBOSE} `go list ./collectors/... | $(GREP) -v test`

.PHONY: integration-test-operators
integration-test-operators: ## Run operators integration tests.
	go test -tags integration -run=Integration ./operators/...

.PHONY: e2e-tests
e2e-tests: tools ## Run E2E tests.
	@echo "Running e2e tests ..."
	@./cicd-scripts/run-e2e-tests.sh

.PHONY: e2e-tests-in-kind
ifeq ($(OPENSHIFT_CI),true)
e2e-tests-in-kind: $(KUSTOMIZE) ## Run E2E tests in a local kind cluster.
	@echo "Running e2e tests in KinD cluster..."
    # Set up environment specific to OpenShift CI
	@IS_KIND_ENV=true SED=$(SED) ./cicd-scripts/run-e2e-in-kind-via-prow.sh
else
e2e-tests-in-kind:
	@echo "Running e2e tests in KinD cluster..."
	@kind get kubeconfig --name hub > /tmp/hub.yaml
	@IS_KIND_ENV=true KUBECONFIG=/tmp/hub.yaml SED=$(SED) ./cicd-scripts/run-e2e-tests.sh
endif

##@ Build dependencies

# Creates a KinD cluster and sets the kubeconfig context to the cluster
.PHONY: kind-env
kind-env: $(KIND) ## Bootstrap Kind envinronment.
	@echo "Setting up KinD cluster"
	@./scripts/bootstrap-kind-env.sh
	@echo "Cluster has been created"
	$(KIND) export kubeconfig --name=hub
	kubectl label node hub-control-plane node-role.kubernetes.io/master=''

# Creates a KinD cluster with MCO deployed and sets the kubeconfig context to the cluster
# This fully prepares the environment for running e2e tests.
.PHONY: mco-kind-env
mco-kind-env: kind-env ## Prepare Kind environment for E2E tests.
	@echo "Local environment has been set up"
	@echo "Installing MCO"
	@$(KIND) get kubeconfig --name hub > /tmp/hub.yaml
	KUBECONFIG=/tmp/hub.yaml IS_KIND_ENV=true ./cicd-scripts/setup-e2e-tests.sh

.PHONY: tools
tools: $(KUSTOMIZE) $(KIND) $(GOJSONTOYAML) ## Install development and e2e tools.
	mkdir -p $(BIN_DIR)
	./scripts/install-binaries.sh install_binaries $(BIN_DIR)

.PHONY: install-envtest-deps
install-envtest-deps: ## Install env-test.
	@mkdir -p $(BIN_DIR)
	@./scripts/install-binaries.sh install_envtest_deps $(BIN_DIR)

.PHONY: install-check-metrics-deps
install-check-metrics-deps:
	@mkdir -p $(BIN_DIR)
	@./scripts/install-binaries.sh install_jq $(BIN_DIR)
	@./scripts/install-binaries.sh install_yq $(BIN_DIR)
	@./scripts/install-binaries.sh install_mimirtool $(BIN_DIR)
	@./scripts/install-binaries.sh install_promtool $(BIN_DIR)

##@ Multi-Cluster-Observability Operator

.PHONY: deploy
deploy:  ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	@$(MAKE) -C operators/multiclusterobservability deploy

# UnDeploy controller from the configured Kubernetes cluster in ~/.kube/config
.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@$(MAKE) -C operators/multiclusterobservability undeploy

# Build the operator binary
.PHONY: build
build: ## Build manager binary.
	@$(MAKE) -C operators/multiclusterobservability build

# Build the docker image
.PHONY: docker-build
docker-build: ## Build docker image with the manager using private RHEL base images.
	@$(MAKE) -C operators/multiclusterobservability docker-build

# Build the docker image using a public image registry
.PHONY: docker-build-local
docker-build-local:  ## Build docker image with the manager using public UBI base images.
	@$(MAKE) -C operators/multiclusterobservability docker-build-local

# Push the docker image
.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	@$(MAKE) -C operators/multiclusterobservability docker-push

.PHONY: bundle
bundle: deps ## Generate bundle manifests and metadata, then validate generated files.
	$(MAKE) -C operators/multiclusterobservability bundle
