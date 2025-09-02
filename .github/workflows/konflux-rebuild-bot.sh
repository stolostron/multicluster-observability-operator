#!/bin/bash

# args: $1: git-sha $2: branch-name
add_commit_comment() {
  SHA=$1
  BRANCH=$2

  jq -nc "{\"body\": \"/retest branch:$BRANCH\"}" |
    curl -sL -X POST -d @- \
      -H "Content-Type: application/json" \
      -H "Authorization: token $GITHUB_TOKEN" \
      "https://api.github.com/repos/$GITHUB_REPOSITORY/commits/$SHA/comments"
}

DEFAULT_BRANCH=$2
GITHUB_TOKEN=$1

# handle main/default branch separately
git checkout "$DEFAULT_BRANCH"
SHA=$(git log -n 1 --pretty=format:"%H" "$DEFAULT_BRANCH")
add_commit_comment "$SHA" "$DEFAULT_BRANCH"

# run on the last 4 branches expect the default one
LATEST_RELEASE=$(git branch -a | grep -o "release-[[:digit:]].\([[:digit:]]*\)" | sed -e "s/^release-//" | sort -V | tail -n 1)
CUR_REL=$LATEST_RELEASE
for _ in {1..4}; do
  CUR_REL=$(echo "$CUR_REL" | awk -F. -v OFS=. '{$NF -= 1 ; print}')
  git checkout release-"$CUR_REL"
  SHA=$(git log -n 1 --pretty=format:"%H" release-"$CUR_REL")
  add_commit_comment "$SHA" "release-$CUR_REL"
done
