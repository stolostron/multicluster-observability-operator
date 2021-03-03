#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# generate csv
./cicd-scripts/install-dependencies.sh

operator-sdk generate csv --crd-dir=deploy/crds --deploy-dir=deploy/ --output-dir=deploy/olm-catalog/multicluster-observability-operator --operator-name=multicluster-observability-operator --csv-version=0.1.0
extra_text="    - name: observatoria.core.observatorium.io
      version: v1alpha1
      kind: Observatorium
      displayName: Observatorium
      description: Observatorium is the Schema for the observatoria API
    - name: observabilityaddons.observability.open-cluster-management.io
      version: v1beta1
      kind: ObservabilityAddon
      displayName: ObservabilityAddon
      description: ObservabilityAddon is the Schema for the observabilityaddon API"
echo "$extra_text" > extra_text_tmp

sed_command='sed -i-e -e'
if [[ "$(uname)" == "Darwin" ]]; then
    sed_command='sed -i '-e' -e'
fi

$sed_command 's/serviceAccountName: open-cluster-management:multicluster-observability-operator/serviceAccountName: multicluster-observability-operator/g' deploy/olm-catalog/multicluster-observability-operator/manifests/multicluster-observability-operator.clusterserviceversion.yaml
$sed_command '/version: v1beta1/r extra_text_tmp' deploy/olm-catalog/multicluster-observability-operator/manifests/multicluster-observability-operator.clusterserviceversion.yaml
rm -rf extra_text_tmp deploy/olm-catalog/multicluster-observability-operator/manifests/multicluster-observability-operator.clusterserviceversion.yaml-e

# check if there is something needs to be committed
diff deploy/req_crds/core.observatorium.io_observatoria.yaml deploy/olm-catalog/multicluster-observability-operator/manifests/core.observatorium.io_observatoria.yaml
if [ $? -ne 0 ]; then
    echo "Failed to check csv: should update observatorium CRD"
    exit 1
fi

diff deploy/req_crds/observability.open-cluster-management.io_observabilityaddon_crd.yaml deploy/olm-catalog/multicluster-observability-operator/manifests/observability.open-cluster-management.io_observabilityaddon_crd.yaml
if [ $? -ne 0 ]; then
    echo "Failed to check csv: should update observabilityaddon CRD"
    exit 1
fi

diff deploy/crds/observability.open-cluster-management.io_multiclusterobservabilities_crd.yaml deploy/olm-catalog/multicluster-observability-operator/manifests/observability.open-cluster-management.io_multiclusterobservabilities_crd.yaml
if [ $? -ne 0 ]; then
    echo "Failed to check csv: should update multiclusterobservability CRD"
    exit 1
fi

if git diff --exit-code deploy/olm-catalog/multicluster-observability-operator/manifests/multicluster-observability-operator.clusterserviceversion.yaml; then
    echo "Check csv successfully"
else
    echo "Failed to check csv"
    exit 1
fi
