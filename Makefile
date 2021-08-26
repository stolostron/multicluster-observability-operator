# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

-include /opt/build-harness/Makefile.prow

# Image URL to use all building/pushing image targets
IMG ?= quay.io/open-cluster-management/multicluster-observability-operator:latest

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
	go test `go list ./operators/... | grep -v test`

unit-tests-loaders:
	go test `go list ./loaders/... | grep -v test`

unit-tests-proxy:
	go test `go list ./proxy/... | grep -v test`

unit-tests-collectors:
	go test `go list ./collectors/... | grep -v test`

.PHONY: e2e-tests

e2e-tests:
	@echo "Running e2e test ..."
	@./cicd-scripts/run-e2e-tests.sh

.PHONY: e2e-tests-in-kind
e2e-tests-in-kind:
	@echo "Running e2e test in KinD ..."
	@./cicd-scripts/run-e2e-in-kind-via-prow.sh

test-e2e-setup:
	@echo "Seting up e2e test environment ..."
ifdef COMPONENT_IMAGE_NAMES
	# override the image for the e2e test
	@./cicd-scripts/setup-e2e-tests.sh -a install -i $(COMPONENT_IMAGE_NAMES)
else
	# fall back to the latest snapshot image from quay.io for the e2e test
	@./cicd-scripts/setup-e2e-tests.sh -a install
endif

test-e2e-clean:
	@echo "Clean e2e test environment ..."
	@./cicd-scripts/setup-e2e-tests.sh -a uninstall

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle:
	cd operators/multiclusterobservability && make bundle
