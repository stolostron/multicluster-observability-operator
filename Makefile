# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

-include /opt/build-harness/Makefile.prow

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
	go test ${VERBOSE} `go list ./operators/... | grep -v test`

unit-tests-loaders:
	go test ${VERBOSE} `go list ./loaders/... | grep -v test`

unit-tests-proxy:
	go test ${VERBOSE} `go list ./proxy/... | grep -v test`

unit-tests-collectors:
	go test `go list ./collectors/... | grep -v test`

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

