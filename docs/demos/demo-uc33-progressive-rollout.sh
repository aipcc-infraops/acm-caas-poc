#!/usr/bin/env bash
# UC-33: ManifestWorkReplicaSet progressive rollout
# Demo: create rollout, check status, update strategy, delete
set -euo pipefail

ROLLOUT="${1:-kueue-v12}"
PLACEMENT="${2:-gpu-placement}"

echo "=== UC-33: Progressive Rollout ==="

echo "--- Step 1: Create progressive rollout ---"
acmlab rollout create "$ROLLOUT" --placement "$PLACEMENT" \
    --strategy Progressive --max-concurrency 2 --max-failures 10%

echo "--- Step 2: List rollouts ---"
acmlab rollout list

echo "--- Step 3: Get rollout status ---"
acmlab rollout get "$ROLLOUT"

echo "--- Step 4: Update strategy to increase concurrency ---"
acmlab rollout update-strategy "$ROLLOUT" --strategy Progressive --max-concurrency 5

echo "--- Step 5: Verify updated strategy ---"
acmlab rollout get "$ROLLOUT"

echo "--- Step 6: Clean up ---"
acmlab rollout delete "$ROLLOUT"

echo ""
echo "=== Demo complete ==="
echo "Rollout $ROLLOUT created with Progressive strategy, updated, and deleted."
