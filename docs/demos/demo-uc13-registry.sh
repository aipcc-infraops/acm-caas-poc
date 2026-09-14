#!/usr/bin/env bash
# UC-13: Registry mirror for restricted clusters (ROKS, air-gap)
# Demo: list images, generate mirror script, configure, wait for placement, check status
# NOTE: Mirror configuration creates a Placement that may take ~30s to reconcile.
set -euo pipefail

CLUSTER="${1:-restricted-cluster}"
MIRROR="${2:-mirror.example.com/acm-mirror}"
PULL_SECRET="${3:-~/mirror-pull-secret.json}"
PLACEMENT_WAIT="${PLACEMENT_WAIT:-30}"

echo "=== UC-13: Registry Mirror ==="

echo "--- Step 1: List images ACM needs on the spoke ---"
acmlab registry list-images "$CLUSTER"

echo "--- Step 2: Generate skopeo mirror script ---"
echo "(Saving to /tmp/mirror-${CLUSTER}.sh)"
acmlab registry mirror-script "$CLUSTER" --target "$MIRROR" > "/tmp/mirror-${CLUSTER}.sh"
echo "Generated $(wc -l < "/tmp/mirror-${CLUSTER}.sh") lines"

echo "--- Step 3: Configure the mirror on the hub ---"
acmlab registry configure "$CLUSTER" \
  --mirror "$MIRROR" \
  --pull-secret "$PULL_SECRET"

echo "--- Step 4: Wait for Placement reconciliation (~${PLACEMENT_WAIT}s) ---"
sleep "$PLACEMENT_WAIT"

echo "--- Step 5: Check mirror status ---"
acmlab registry status "$CLUSTER"

echo "--- Cleanup ---"
echo "To remove: acmlab registry remove $CLUSTER"
