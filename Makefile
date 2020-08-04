include ./cicd-scripts/Configfile

-include $(shell curl -H 'Authorization: token ${GITHUB_TOKEN}' -H 'Accept: application/vnd.github.v4.raw' -L https://api.github.com/repos/open-cluster-management/build-harness-extensions/contents/templates/Makefile.build-harness-bootstrap -o .build-harness-bootstrap; echo .build-harness-bootstrap)

docker-binary:
	CGO_ENABLED=0 go build -a -installsuffix cgo -v -i -o build/_output/bin/multicluster-observability-operator github.com/open-cluster-management/multicluster-monitoring-operator/cmd/manager

copyright-check:
	./cicd-scripts/copyright-check.sh $(TRAVIS_BRANCH)

unit-tests:
	@echo "TODO: Run unit-tests"
	go test ./... -v -coverprofile cover.out
	go tool cover -html=cover.out -o=cover.html

e2e-tests:
	@echo "TODO: Run e2e-tests"