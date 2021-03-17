# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

include ./cicd-scripts/Configfile

-include $(shell curl -H 'Authorization: token ${GITHUB_TOKEN}' -H 'Accept: application/vnd.github.v4.raw' -L https://api.github.com/repos/open-cluster-management/build-harness-extensions/contents/templates/Makefile.build-harness-bootstrap -o .build-harness-bootstrap; echo .build-harness-bootstrap)

CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

docker-binary:
	CGO_ENABLED=0 go build -a -installsuffix cgo -v -i -o bin/manager main.go

copyright-check:
	./cicd-scripts/copyright-check.sh $(TRAVIS_BRANCH)

unit-tests:
	@echo "TODO: Run unit-tests"
	go test ./... -v -coverprofile cover.out
	go tool cover -html=cover.out -o=cover.html

e2e-tests:
	@echo "TODO: Run e2e-tests"

# should set the correct IMAGE_TAG and IMAGE_NAME for the new csv
update-csv:
	./cicd-scripts/update-check-mco-csv.sh

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Download controller-gen locally if necessary
CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen:
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
