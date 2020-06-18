#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.

echo "<repo>/<component>:<tag> : $1"

git config --global url."https://$GITHUB_TOKEN@github.com/open-cluster-management".insteadOf "https://github.com/open-cluster-management"

go test ./...