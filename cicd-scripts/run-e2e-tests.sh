#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.

echo "<repo>/<component>:<tag> : $1"

./tests/e2e/setup.sh $1
if [ $? -ne 0 ]; then
    echo "Cannot setup environment successfully."
    exit 2
fi

./tests/e2e/tests.sh
if [ $? -ne 0 ]; then
    echo "Cannot pass all test cases."
    exit 2
fi
