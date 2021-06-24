# Copyright Contributors to the Open Cluster Management project

-include /opt/build-harness/Makefile.prow

.PHONY: build
build:
	go build -o main ./cmd/main.go

unit-tests:
	go test `go list ./... | grep -v test`

e2e-tests:
	@./cicd-scripts/run-e2e-tests.sh
