#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

require_env \
  INSTALLER_DEV_TOOLS_DIR \
  TARGET_CSV_VERSION \
  FROM_CSV_VERSION \
  BASE_ACM \
  BASE_CATALOG_TAG \
  ACM_IMG_TAG \
  COMPONENT_BRANCH

require_tool just

# would need to run from installer-dev-tools
opm_policy_file="$(mktemp "${TMPDIR:-/tmp}/opm-policy.XXXXXX")"
trap 'rm -f "$opm_policy_file"' EXIT
printf '%s\n' '{"default":[{"type":"reject"}],"transports":{"docker":{"quay.io":[{"type":"insecureAcceptAnything"}]}}}' >"$opm_policy_file"

(
  cd "$INSTALLER_DEV_TOOLS_DIR"
  TARGET_PLATFORM=linux/amd64 CONTAINERS_POLICY_JSON="$opm_policy_file" \
    just dev-branch-e2e \
    --username "$QUAY_USER" \
    --branch main \
    --operator-branch "$BASE_ACM" \
    --bundle-branch "$BASE_ACM" \
    --base-catalog-tag "$BASE_CATALOG_TAG" \
    --component-branches "$COMPONENT_BRANCH" \
    --csv-version "$TARGET_CSV_VERSION" \
    --upgrade-from-csv "$FROM_CSV_VERSION" \
    --image-tag "$ACM_IMG_TAG" \
    --use-https
)
