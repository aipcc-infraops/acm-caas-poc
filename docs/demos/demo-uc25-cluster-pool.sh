#!/usr/bin/env bash
# UC-25: ClusterPool and ClusterClaim — pre-warmed clusters
# Demo: create pool, claim cluster, release claim
set -euo pipefail

POOL="${1:-amd64-419}"

echo "=== UC-25: ClusterPool & ClusterClaim ==="

echo "--- Step 1: Create cluster pool ---"
acmlab pool create "$POOL" --size 3 --image-set img4.19-multi \
    --platform ibmcloud --region us-south --base-domain example.com

echo "--- Step 2: List pools ---"
acmlab pool list

echo "--- Step 3: Get pool details ---"
acmlab pool get "$POOL"

echo "--- Step 4: Claim a cluster ---"
acmlab claim create "$POOL" --name my-test --ttl 48h

echo "--- Step 5: List claims ---"
acmlab claim list

echo "--- Step 6: Release the claim ---"
acmlab claim release my-test

echo "--- Step 7: Clean up pool ---"
acmlab pool delete "$POOL"

echo ""
echo "=== Demo complete ==="
echo "Pool $POOL created, cluster claimed, released, and pool deleted."
