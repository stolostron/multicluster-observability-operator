#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

echo "<repo>/<component>:<tag> : $1"

git config --global url."https://$GITHUB_TOKEN@github.com/open-cluster-management".insteadOf "https://github.com/open-cluster-management"

go test ./...