#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.
set -e

make docker-binary

git config --global url."https://$GITHUB_TOKEN@github.com/open-cluster-management".insteadOf  "https://github.com/open-cluster-management"

echo "Building multicluster-monitoring-operator image"
export DOCKER_IMAGE_AND_TAG=${1}
export DOCKER_FILE=Dockerfile
make docker/build