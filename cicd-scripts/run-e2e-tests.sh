#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.

set -o errexi
set -o nounset
set -o pipefail
set -o xtrace

echo "<repo>/<component>:<tag> : $1"

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
