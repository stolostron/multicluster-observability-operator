#!/bin/bash
# Copyright (c) 2020 Red Hat, Inc.

echo "<repo>/<component>:<tag> : $1"

git config --global url."https://$GITHUB_TOKEN@github.com/open-cluster-management".insteadOf  "https://github.com/open-cluster-management"

WORKDIR=`pwd`
cd ${WORKDIR}/..
git clone https://github.com/open-cluster-management/observability-kind-cluster.git
cd observability-kind-cluster
./setup.sh $1
if [ $? -ne 0 ]; then
    echo "Cannot setup environment successfully."
    exit 1
fi

cd ${WORKDIR}/..
git clone https://github.com/open-cluster-management/observability-e2e-test.git
cd observability-e2e-test

go get -u github.com/onsi/ginkgo/ginkgo

export KUBECONFIG=$HOME/.kube/kind-config-hub
export SKIP_INSTALL_STEP=true

printf "options:" >> resources/options.yaml
printf "\n  hub:" >> resources/options.yaml
printf "\n    baseDomain: placeholder" >> resources/options.yaml
printf "\n    masterURL: https://127.0.0.1:32806" >> resources/options.yaml
printf "\n    grafanaURL: http://127.0.0.1" >> resources/options.yaml
printf "\n    grafanaHost: grafana-test" >> resources/options.yaml

ginkgo -v -- -options=resources/options.yaml -v=3

cat results.xml | grep failures=\"0\" | grep errors=\"0\"
if [ $? -ne 0 ]; then
    echo "Cannot pass all test cases."
    cat results.xml
    exit 1
fi
