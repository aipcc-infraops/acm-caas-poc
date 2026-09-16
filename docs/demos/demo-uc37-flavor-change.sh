#!/usr/bin/env bash
# UC-37: Worker node flavor change via MachinePool rolling replacement
# Demo: change worker instance type on a Hive-provisioned cluster
set -euo pipefail

CLUSTER="${1:-spoke1}"
NEW_TYPE="${2:-cx2-8x16}"

echo "=== UC-37: Worker Node Flavor Change ==="

echo "--- Step 1: Check current MachinePool ---"
acmlab scaling get "$CLUSTER"

echo "--- Step 2: Change worker flavor to $NEW_TYPE ---"
acmlab scaling set-flavor "$CLUSTER" --worker-type "$NEW_TYPE"

echo "--- Step 3: Verify updated MachinePool ---"
acmlab scaling get "$CLUSTER"

echo "=== Demo complete ==="
echo "Hive will perform a rolling replacement of worker nodes."
echo "Monitor progress with: acmlab scaling get $CLUSTER"
