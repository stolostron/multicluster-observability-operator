#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.

echo "<repo>/<component>:<tag> : $1"

git config --global url."https://$GITHUB_TOKEN@github.com/open-cluster-management".insteadOf  "https://github.com/open-cluster-management"

./tests/e2e/setup.sh $1
if [ $? -ne 0 ]; then
    echo "Cannot setup environment successfully."
    exit 1
fi

./tests/e2e/tests.sh
if [ $? -ne 0 ]; then
    echo "Cannot pass all test cases."
    exit 1
fi
