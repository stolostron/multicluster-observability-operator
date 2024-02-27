#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

echo "install dependencies"

_OPERATOR_SDK_VERSION=v1.4.2

if ! [ -x "$(command -v operator-sdk)" ]; then
  curl -L "https://github.com/operator-framework/operator-sdk/releases/download/${_OPERATOR_SDK_VERSION}/operator-sdk_$(uname | tr '[:upper:]' '[:lower:]')_$(uname -p)64" -o operator-sdk
  chmod +x operator-sdk
  sudo mv operator-sdk /usr/local/bin/operator-sdk
fi
