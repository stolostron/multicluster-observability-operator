include ./cicd-scripts/Configfile

-include $(shell curl -H 'Authorization: token ${GITHUB_TOKEN}' -H 'Accept: application/vnd.github.v4.raw' -L https://api.github.com/repos/stolostron/build-harness-extensions/contents/templates/Makefile.build-harness-bootstrap -o .build-harness-bootstrap; echo .build-harness-bootstrap)

docker-binary:
	CGO_ENABLED=0 go build -a -installsuffix cgo -v -i -o build/_output/bin/multicluster-monitoring-operator github.com/stolostron/multicluster-monitoring-operator/cmd/manager

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
