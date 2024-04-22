#!/bin/bash
# Copyright (c) 2024 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Use snapshot for target release.
# Use latest if no branch info detected, or not a release branch.
VERSION="2.11"

# Determine, based on the branch, which image to use for the test.
# If the branch is main, then assume that an image for the PR has been built and pushed to quay.io.
# If the branch is a release branch, then use the latest snapshot image for that release.
get_container_image() {
  BRANCH=""
  LATEST_SNAPSHOT=""

  if [[ ${PULL_BASE_REF} == "main" ]]; then
    LATEST_SNAPSHOT="${VERSION}-PR${PULL_NUMBER}-${PULL_PULL_SHA}"
  else
    LATEST_SNAPSHOT=$(get_latest_snapshot)
  fi

  # trim the leading and tailing quotes
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT#\"}"
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT%\"}"
  echo ${LATEST_SNAPSHOT}
}

get_latest_snapshot() {
  BRANCH=""
  LATEST_SNAPSHOT=""
  SNAPSHOT_RELEASE=${SNAPSHOT_RELEASE:=$VERSION}
  MATCH=$SNAPSHOT_RELEASE".*-SNAPSHOT"
  if [[ ${PULL_BASE_REF} == "release-"* ]]; then
    BRANCH=${PULL_BASE_REF#"release-"}
    LATEST_SNAPSHOT=$(curl https://quay.io/api/v1/repository/open-cluster-management/multicluster-observability-operator | jq '.tags|with_entries(select(.key|test("'${BRANCH}'.*-SNAPSHOT-*")))|keys[length-1]') fi
  if [[ ${LATEST_SNAPSHOT} == "null" ]] || [[ ${LATEST_SNAPSHOT} == "" ]]; then
    LATEST_SNAPSHOT=$(curl https://quay.io/api/v1/repository/stolostron/multicluster-observability-operator/tag/ | jq --arg MATCH "$MATCH" '.tags[] | select(.name | match($MATCH; "i")  ).name' | sort -r --version-sort | head -n 1)
  fi

  # trim the leading and tailing quotes
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT#\"}"
  LATEST_SNAPSHOT="${LATEST_SNAPSHOT%\"}"
  echo ${LATEST_SNAPSHOT}
}
