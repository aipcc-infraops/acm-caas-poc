#!/usr/bin/env bash
# UC-15: Resource quota gates via governance policy
# Demo: apply quota limits and check compliance
set -euo pipefail

CLUSTER="${1:-spoke2}"

echo "=== UC-15: Resource Quota Enforcement ==="

echo "--- Step 1: Apply worker quota ---"
acmlab policy apply-quota --cluster "$CLUSTER" --max-workers 5

echo "--- Step 2: Apply GPU quota ---"
acmlab policy apply-quota --cluster "$CLUSTER" --max-workers 5 --max-gpus 1

echo "--- Step 3: Check quota status ---"
acmlab policy quota-status "$CLUSTER"

echo "--- Step 4: Verify policy exists ---"
acmlab policy list

echo ""
echo "=== Demo complete ==="
echo "Cluster $CLUSTER now has quota limits: max-workers=5, max-gpus=1"
echo "The ConfigurationPolicy will mark the cluster NonCompliant if limits are exceeded."
