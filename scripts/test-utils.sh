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
  LATEST_SNAPSHOT=$(curl -s "https://quay.io/api/v1/repository/stolostron/multicluster-observability-operator/tag/?filter_tag_name=like:$SNAPSHOT_RELEASE&limit=100" | jq --arg MATCH "$MATCH" '.tags[] | select(.name | match($MATCH; "i")  ).name' | sort -r --version-sort | head -n 1)

  # trim the leading and tailing quotes
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT#\"}"
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT%\"}"

  # try previous version if none were found.
  # this makes builds work during branching times.
  # we only go back once
  if [[ -z $LATEST_SNAPSHOT ]] && [[ -z $PREVIOUS_VERSION ]]; then
    PREVIOUS_VERSION=$(bc -l <<<"$SNAPSHOT_RELEASE-0.01")
    echo >&2 "****WARNING**** Attempting to use previous ACM version $PREVIOUS_VERSION as no snapshots found for $SNAPSHOT_RELEASE"
    LATEST_SNAPSHOT=$(SNAPSHOT_RELEASE=$PREVIOUS_VERSION get_latest_acm_snapshot)
  fi

  if [[ -z $LATEST_SNAPSHOT ]]; then
    echo >&2 "****ERROR**** Unable to find ACM image for release: $SNAPSHOT_RELEASE"
  fi

  echo "${LATEST_SNAPSHOT}"
}

get_latest_mce_snapshot() {
  SNAPSHOT_RELEASE=${SNAPSHOT_RELEASE:=$VERSION}
  # matches:
  # version number (i.e 2.11)
  # z version 1-2 digits
  # -SNAPSHOT
  MATCH=$SNAPSHOT_RELEASE".\d{1,2}-SNAPSHOT"
  LATEST_SNAPSHOT=$(curl -s "https://quay.io/api/v1/repository/stolostron/registration/tag/?filter_tag_name=like:$SNAPSHOT_RELEASE&limit=100" | jq --arg MATCH "$MATCH" '.tags[] | select(.name | match($MATCH; "i")  ).name' | sort -r --version-sort | head -n 1)

  # trim the leading and tailing quotes
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT#\"}"
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT%\"}"

  # try previous version if none were found.
  # this makes builds work during branching times.
  # we only go back once
  if [[ -z $LATEST_SNAPSHOT ]] && [[ -z $PREVIOUS_VERSION ]]; then
    PREVIOUS_VERSION=$(bc -l <<<"$SNAPSHOT_RELEASE-0.01")
    echo >&2 "****WARNING**** Attempting to use previous MCE version $PREVIOUS_VERSION as no snapshots found for $SNAPSHOT_RELEASE"
    LATEST_SNAPSHOT=$(SNAPSHOT_RELEASE=$PREVIOUS_VERSION get_latest_mce_snapshot)
  fi

  if [[ -z $LATEST_SNAPSHOT ]]; then
    echo >&2 "****ERROR**** Unable to find MCE image for release: $SNAPSHOT_RELEASE"
  fi

  echo "${LATEST_SNAPSHOT}"
}
