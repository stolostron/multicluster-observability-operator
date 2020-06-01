#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.

echo "<repo>/<component>:<tag> : $1"

git config credential.helper store
git config user.name ${GITHUB_USER}
echo "https://${GITHUB_TOKEN}:x-oauth-basic@github.com" >> ~/.git-credentials
git config -l

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
