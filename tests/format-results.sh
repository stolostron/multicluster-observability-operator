# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Format the result to add Observability label for BeforeSuite and AfterSuite
# Log a requirement in ginkgo - https://github.com/onsi/ginkgo/issues/795

#!/bin/bash

if [ -z $1 ]; then
    echo "Please provide the results file."
    exit 1
fi

sed -i "s~BeforeSuite~Observability: [P1][Sev1][Observability] Cannot enable observability service successfully~g" $1
sed -i "s~AfterSuite~Observability: [P1][Sev1][Observability] Cannot uninstall observability service completely~g" $1
