#!/usr/bin/env bash

set -euo pipefail

# Required: ACM_IMG, PUBLISHER, TARGET_CSV_VERSION (for example, v2.17.4-managedlogstore)
# Optional: ACM_CHANNEL (defaults to the installed ACM major.minor release channel)
#           CATALOG_SOURCE_NAME (defaults to acm-managed-logstore-<major><minor>)
# Happy path
ACM_IMG='quay.io/rh-ee-dbuchana/acm-dev-catalog:managed-logstore-test-5.0-amd64'
PUBLISHER='cicd'
TARGET_CSV='advanced-cluster-management.v5.0.1-managedlogstore'

NAMESPACE=open-cluster-management
CATALOG_NAMESPACE=openshift-marketplace

SUB=$(oc -n "$NAMESPACE" get subscriptions.operators.coreos.com -o json |
  jq -er '[.items[] | select(.spec.name == "advanced-cluster-management")] |
    if length == 1 then .[0].metadata.name else error("expected one ACM subscription") end')

CURRENT_CSV=$(oc -n "$NAMESPACE" get subscriptions.operators.coreos.com "$SUB" \
  -o jsonpath='{.status.currentCSV}')
if [[ $CURRENT_CSV =~ ^advanced-cluster-management\.v([0-9]+\.[0-9]+)\. ]]; then
  ACM_MAJOR_MINOR=${BASH_REMATCH[1]}
else
  printf 'Could not determine ACM major.minor from current CSV: %s\n' "$CURRENT_CSV" >&2
  # should we exit here?
  exit 1
fi
# current version mismatch, not so happy path
ACM_CHANNEL=${ACM_CHANNEL:-release-$ACM_MAJOR_MINOR}
CATALOG_SOURCE_NAME=${CATALOG_SOURCE_NAME:-acm-managed-logstore-${ACM_MAJOR_MINOR//./}}

printf 'ACM Subscription: %s\n' "$SUB"
printf 'Installed ACM CSV: %s (channel: %s)\n' "$CURRENT_CSV" "$ACM_CHANNEL"
printf 'Target ACM CSV: %s\n' "$TARGET_CSV"

jq -n \
  --arg name "$CATALOG_SOURCE_NAME" \
  --arg namespace "$CATALOG_NAMESPACE" \
  --arg publisher "$PUBLISHER" \
  --arg image "$ACM_IMG" \
  '{apiVersion:"operators.coreos.com/v1alpha1",kind:"CatalogSource",
    metadata:{name:$name,namespace:$namespace},
    spec:{displayName:"ACM managed logstore test",publisher:$publisher,
      sourceType:"grpc",image:$image}}' |
  oc apply -f -

oc -n "$CATALOG_NAMESPACE" wait \
  "catalogsources.operators.coreos.com/$CATALOG_SOURCE_NAME" \
  --for=jsonpath='{.status.connectionState.lastObservedState}'=READY \
  --timeout=5m

PATCH=$(jq -nc \
  --arg source "$CATALOG_SOURCE_NAME" \
  --arg namespace "$CATALOG_NAMESPACE" \
  --arg channel "$ACM_CHANNEL" \
  '{spec:{source:$source,sourceNamespace:$namespace,channel:$channel,installPlanApproval:"Automatic"}}')
oc -n "$NAMESPACE" patch subscriptions.operators.coreos.com \
  "$SUB" --type=merge -p "$PATCH"

oc -n "$NAMESPACE" get subscriptions.operators.coreos.com \
  "$SUB" -o jsonpath='{.spec.source}{" "}{.spec.installPlanApproval}{"\n"}'

oc -n "$NAMESPACE" wait \
  --for=create "clusterserviceversions.operators.coreos.com/$TARGET_CSV" \
  --timeout=10m

oc -n "$NAMESPACE" wait \
  "clusterserviceversions.operators.coreos.com/$TARGET_CSV" \
  --for=jsonpath='{.status.phase}'=Succeeded \
  --timeout=20m

oc -n "$NAMESPACE" get installplans.operators.coreos.com -o json |
  jq -r --arg csv "$TARGET_CSV" \
    '.items[] | select((.spec.clusterServiceVersionNames // []) | index($csv)) |
     [.metadata.name, .spec.approval, (.spec.approved | tostring), (.status.phase // "")] | @tsv'

oc -n "$NAMESPACE" rollout status \
  deployment/multiclusterhub-operator --timeout=10m

oc -n "$NAMESPACE" wait \
  multiclusterhub.operator.open-cluster-management.io/multiclusterhub \
  --for=jsonpath='{.status.phase}'=Running --timeout=15m

oc -n "$NAMESPACE" wait \
  --for=create deployment/multicluster-observability-operator --timeout=10m

oc -n "$NAMESPACE" rollout status \
  deployment/multicluster-observability-operator --timeout=10m

oc -n "$NAMESPACE" get deployment \
  multiclusterhub-operator multicluster-observability-operator \
  -o 'custom-columns=NAME:.metadata.name,IMAGE:.spec.template.spec.containers[*].image'
