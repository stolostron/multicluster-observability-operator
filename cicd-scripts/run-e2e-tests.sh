#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

git clone --depth 1 https://github.com/open-cluster-management/observability-e2e-test.git
cd observability-e2e-test
make test-e2e
