# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

-include /opt/build-harness/Makefile.prow
include .bingo/Variables.mk

FILES_TO_FMT ?= $(shell find . -path ./vendor -prune -o -name '*.deepcopy.go' -prune -o -name '*.go' -print)
TMP_DIR := $(shell pwd)/tmp
BIN_DIR ?= $(TMP_DIR)/bin
GIT ?= $(shell which git)

XARGS ?= $(shell which gxargs 2>/dev/null || which xargs)
GREP ?= $(shell which ggrep 2>/dev/null || which grep)

# Image URL to use all building/pushing image targets
IMG ?= quay.io/stolostron/multicluster-observability-operator:latest

KUSTOMIZE_VERSION ?= v5.3.0

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


LOCAL_IMAGE ?= hack.io/stolostron/mco:local
IMAGE_BUILD_CMD ?= docker buildx build . -t ${LOCAL_IMAGE} -f operators/multiclusterobservability/Dockerfile.dev --load

# Build the docker image using a public image registry
# This target currently supports docker desktop by default
.PHONY: docker-build-local
docker-build-local:
	cd operators/multiclusterobservability && make manager
	$(IMAGE_BUILD_CMD)

# Create a local environment for development in KinD
.PHONY: local-env
local-env: docker-build-local local-kind-cluster
	@kind load docker-image ${LOCAL_IMAGE} --name=hub
	@echo "Setting up local environment"
	@./tests/run-in-kind/install-dependencies.sh deploy_all
	@echo "Local environment has been set up"
	@echo "Installing MCO"
	@kind get kubeconfig --name hub > /tmp/hub.yaml
	KUBECONFIG=/tmp/hub.yaml MULTICLUSTER_OBSERVABILITY_OPERATOR_IMAGE_REF=${LOCAL_IMAGE} \
		IS_KIND_ENV=true KUSTOMIZE_VERSION=${KUSTOMIZE_VERSION} ./cicd-scripts/setup-e2e-tests.sh


# Create a local KinD cluster and sets the kubeconfig context to the cluster
.PHONY: local-kind-cluster
local-kind-cluster:
	@echo "Setting up KinD cluster"
	@./tests/run-in-kind/create-cluster.sh create_kind_cluster hub
	@echo "Cluster has been created"
	kind export kubeconfig --name=hub
	kubectl label node hub-control-plane node-role.kubernetes.io/master=''

# Push the docker image
docker-push:
	docker push ${IMG}


.PHONY: unit-tests
unit-tests: unit-tests-operators unit-tests-loaders unit-tests-proxy unit-tests-collectors

unit-tests-operators:
	go test -v ${VERBOSE} `go list ./operators/... | $(GREP) -v test`

unit-tests-loaders:
	go test -v ${VERBOSE} `go list ./loaders/... | $(GREP) -v test`

unit-tests-proxy:
	go test -v ${VERBOSE} `go list ./proxy/... | $(GREP) -v test`

unit-tests-collectors:
	go test ${VERBOSE} `go list ./collectors/... | $(GREP) -v test`

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

# PROTIP:
# Add
#      --cpu-profile-path string   Path to CPU profile output file
#      --mem-profile-path string   Path to memory profile output file
# to debug big allocations during linting.
.PHONY: lint
lint: check-git deps format $(GOLANGCI_LINT) $(FAILLINT)
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
io/ioutil.{Discard,NopCloser,ReadAll,ReadDir,ReadFile,TempDir,TempFile,Writefile}" ./...
	@$(FAILLINT) -paths "fmt.{Print,Println}" -ignore-tests ./...
	@echo ">> examining all of the Go files"
	@go vet -stdmethods=false ./...
	@echo ">> linting all of the Go files GOGC=${GOGC}"
	@$(GOLANGCI_LINT) run
	@echo ">> ensuring Copyright headers"
	@go run ./scripts/copyright
	$(call require_clean_work_tree,'detected files without copyright, run make lint and commit changes')
