#!/bin/bash
# Copyright (c) 2024 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Use snapshot for target release.
# Use latest if no branch info detected, or not a release branch.
get_latest_snapshot() {
  BRANCH=""
  LATEST_SNAPSHOT=""
  SNAPSHOT_RELEASE=${SNAPSHOT_RELEASE:=2.10}
  MATCH=$SNAPSHOT_RELEASE".*-SNAPSHOT"
  if [[ ${PULL_BASE_REF} == "release-"* ]]; then
    BRANCH=${PULL_BASE_REF#"release-"}
    LATEST_SNAPSHOT=$(curl https://quay.io//api/v1/repository/open-cluster-management/multicluster-observability-operator | jq '.tags|with_entries(select(.key|test("'"${BRANCH}"'.*-SNAPSHOT-*")))|keys[length-1]')
  fi
  if [[ ${LATEST_SNAPSHOT} == "null" ]] || [[ ${LATEST_SNAPSHOT} == "" ]]; then
    LATEST_SNAPSHOT=$(curl "https://quay.io/api/v1/repository/stolostron/multicluster-observability-operator/tag/?filter_tag_name=like:$SNAPSHOT_RELEASE&limit=100" | jq --arg MATCH "$MATCH" '.tags[] | select(.name | match($MATCH; "i")  ).name' | sort -r --version-sort | head -n 1)
  fi

  # trim the leading and tailing quotes
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT#\"}"
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT%\"}"
  echo "${LATEST_SNAPSHOT}"
}
