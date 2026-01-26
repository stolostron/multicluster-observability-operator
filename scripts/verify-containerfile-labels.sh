#!/usr/bin/env bash
# Copyright Contributors to the Open Cluster Management project
# Copyright (c) 2026 Red Hat, Inc.

set -euo pipefail

# Verifies that RHEL versions are consistent across FROM statements, name labels, and cpe labels
# in all Containerfile.operator files. This prevents mismatches when updating RHEL versions.
#
# Usage: ./scripts/verify-containerfile-labels.sh
# Exit code: 0 if all checks pass, 1 if any mismatch detected

echo "Verifying Containerfile.operator label consistency..."
echo ""

FAILED=0

# Find all production Containerfile.operator files (exclude vendor/, tests/, tools/)
# Only check production images that are released to registry.redhat.io
CONTAINERFILES=$(find . -name "Containerfile.operator" \
  -not -path "*/vendor/*" \
  -not -path "*/tests/*" \
  -not -path "*/tools/*" \
  | sort)

if [ -z "$CONTAINERFILES" ]; then
  echo "ERROR: No Containerfile.operator files found"
  exit 1
fi

for containerfile in $CONTAINERFILES; do
  echo "Checking $containerfile..."

  # Extract RHEL version from FROM statement (e.g., "FROM ubi9/ubi-minimal" -> "9")
  from_rhel=$(grep "^FROM.*ubi" "$containerfile" | sed -E 's/.*ubi([0-9]+).*/\1/' || echo "")

  # Extract RHEL version from name label (e.g., "name=rhacm2/foo-rhel9-operator" -> "9")
  name_rhel=$(grep 'name="rhacm2/.*-rhel' "$containerfile" | sed -E 's/.*-rhel([0-9]+).*/\1/' || echo "")

  # Extract RHEL version from cpe label (e.g., "cpe=cpe:/a:redhat:acm:2.16::el9" -> "9")
  cpe_rhel=$(grep 'cpe="cpe:/a:redhat:acm' "$containerfile" | sed -E 's/.*::el([0-9]+).*/\1/' || echo "")

  # Validate all versions were extracted
  if [ -z "$from_rhel" ]; then
    echo "  ❌ ERROR: Could not extract RHEL version from FROM statement"
    FAILED=1
    continue
  fi

  if [ -z "$name_rhel" ]; then
    echo "  ❌ ERROR: Could not extract RHEL version from name label"
    FAILED=1
    continue
  fi

  if [ -z "$cpe_rhel" ]; then
    echo "  ❌ ERROR: Could not extract RHEL version from cpe label"
    FAILED=1
    continue
  fi

  # Check for consistency
  if [ "$from_rhel" != "$name_rhel" ] || [ "$from_rhel" != "$cpe_rhel" ]; then
    echo "  ❌ MISMATCH: FROM uses ubi${from_rhel}, name uses rhel${name_rhel}, cpe uses el${cpe_rhel}"
    FAILED=1
  else
    echo "  ✅ OK: All labels consistent with RHEL ${from_rhel}"
  fi

  # Verify name label format (must not use $IMAGE_NAME variable)
  if grep -q 'name="\$IMAGE_NAME"' "$containerfile"; then
    echo "  ❌ ERROR: name label uses \$IMAGE_NAME variable instead of hardcoded value"
    FAILED=1
  fi

  # Verify cpe label exists and has correct format
  if ! grep -q 'cpe="cpe:/a:redhat:acm:' "$containerfile"; then
    echo "  ❌ ERROR: cpe label missing or has incorrect format"
    FAILED=1
  fi

  echo ""
done

if [ $FAILED -eq 0 ]; then
  echo "✅ All Containerfile.operator files are consistent"
  exit 0
else
  echo "❌ Verification failed. Please fix the issues above."
  echo ""
  echo "When updating RHEL versions, you must update:"
  echo "  1. FROM registry.access.redhat.com/ubi{VERSION}/ubi-minimal:latest"
  echo "  2. name=\"rhacm2/{component}-rhel{VERSION}-operator\""
  echo "  3. cpe=\"cpe:/a:redhat:acm:{ACM_VER}::el{VERSION}\""
  exit 1
fi
