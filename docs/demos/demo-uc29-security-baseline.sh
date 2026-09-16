#!/usr/bin/env bash
# UC-29: Security baseline enforcement via Gatekeeper/OPA
# Demo: apply CIS Level 1 constraints, check status, remove
set -euo pipefail

CLUSTER="${1:-spoke2}"

echo "=== UC-29: Security Baseline Enforcement ==="

echo "--- Step 1: Apply CIS Level 1 baseline ---"
acmlab security apply cis-level1 --cluster "$CLUSTER"

echo "--- Step 2: Check baseline status ---"
acmlab security status "$CLUSTER"

echo "--- Step 3: List all baselines ---"
acmlab security list

echo "--- Step 4: Verify health policy ---"
acmlab policy list

echo "--- Step 5: Clean up ---"
acmlab security remove "$CLUSTER"

echo ""
echo "=== Demo complete ==="
echo "Cluster $CLUSTER had CIS Level 1 Gatekeeper constraints deployed and removed."
echo "Constraints enforced: no privileged pods, no hostPID/hostNetwork,"
echo "required resource limits, approved image registries only."
