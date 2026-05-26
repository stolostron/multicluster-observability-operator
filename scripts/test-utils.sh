#!/bin/bash
# Copyright (c) 2024 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

WORKDIR="$(
  cd "$(dirname "$0")" || exit
  pwd -P
)"

# Gets version from the component version without the z, i.e 2.11
VERSION=$(awk -F '.' '{ print $1"."$2 }' <"$WORKDIR"/../COMPONENT_VERSION)

# Tries to get the latest snapshot for the current release
# Note if there are more than 100 snapshots per release
# we might not get the latest as the quay API can only return
# 100 results, and we only look at the first page.
get_latest_acm_snapshot() {
  SNAPSHOT_RELEASE=${SNAPSHOT_RELEASE:=$VERSION}
  # matches:
  # version number (i.e 2.11)
  # z version 1-2 digits
  # -SNAPSHOT
  MATCH=$SNAPSHOT_RELEASE".\d{1,2}-SNAPSHOT"
  LATEST_SNAPSHOT=$(curl "https://quay.io/api/v1/repository/stolostron/multicluster-observability-operator/tag/?filter_tag_name=like:$SNAPSHOT_RELEASE&limit=100" | jq --arg MATCH "$MATCH" '.tags[] | select(.name | match($MATCH; "i")  ).name' | sort -r --version-sort | head -n 1)

  # trim the leading and tailing quotes
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT#\"}"
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT%\"}"
  echo "${LATEST_SNAPSHOT}"
}

get_latest_mce_snapshot() {
  SNAPSHOT_RELEASE=${SNAPSHOT_RELEASE:=$VERSION}
  # matches:
  # version number (i.e 2.11)
  # z version 1-2 digits
  # -SNAPSHOT
  MATCH=$SNAPSHOT_RELEASE".\d{1,2}-SNAPSHOT"
  LATEST_SNAPSHOT=$(curl "https://quay.io/api/v1/repository/stolostron/registration/tag/?filter_tag_name=like:$SNAPSHOT_RELEASE&limit=100" | jq --arg MATCH "$MATCH" '.tags[] | select(.name | match($MATCH; "i")  ).name' | sort -r --version-sort | head -n 1)

  # trim the leading and tailing quotes
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT#\"}"
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT%\"}"
  echo "${LATEST_SNAPSHOT}"
}
