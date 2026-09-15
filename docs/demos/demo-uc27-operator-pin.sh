#!/usr/bin/env bash
# UC-27: Operator version pinning via OperatorPolicy
# Demo: pin an operator version, check compliance, update pin
set -euo pipefail

OPERATOR="${1:-gpu-sharing-operator}"
VERSION="${2:-2.17.3}"
CHANNEL="${3:-stable-2.17}"

echo "=== UC-27: Operator Version Pinning ==="

echo "--- Step 1: Apply OperatorPolicy to pin $OPERATOR to $VERSION ---"
acmlab policy apply "pin-${OPERATOR}" \
  --operator "$OPERATOR" \
  --operator-version "$VERSION" \
  --operator-channel "$CHANNEL" \
  --labels "gpu=true"

echo "--- Step 2: Check compliance status ---"
acmlab policy status "pin-${OPERATOR}"

echo "--- Step 3: List all policies ---"
acmlab policy list

echo "--- Step 4: Update pin to new version (example) ---"
echo "  acmlab policy remove pin-${OPERATOR}"
echo "  acmlab policy apply pin-${OPERATOR} --operator $OPERATOR --operator-version 2.18.0 --operator-channel stable-2.18"

echo "--- Step 5: Clean up ---"
echo "  acmlab policy remove pin-${OPERATOR}"

echo "=== Demo complete ==="
