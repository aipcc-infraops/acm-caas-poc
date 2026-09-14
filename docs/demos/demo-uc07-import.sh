#!/usr/bin/env bash
# UC-07: External cluster import and detach
# Demo: import an external cluster, check status, detach, reimport
set -euo pipefail

CLUSTER="${1:-external-cluster}"
KUBECONFIG_PATH="${2:-/tmp/external.kubeconfig}"

echo "=== UC-07: External Cluster Import ==="

echo "--- Step 1: Import cluster with auto-import ---"
acmlab import cluster "$CLUSTER" \
  --kubeconfig-path "$KUBECONFIG_PATH" \
  --label cloud=External \
  --label env=staging \
  --wait --timeout 10m

echo "--- Step 2: Check import status ---"
acmlab import status "$CLUSTER"

echo "--- Step 3: List all imported clusters ---"
acmlab import list

echo "--- Step 4: Verify in fleet ---"
acmlab fleet status "$CLUSTER"

echo "--- Step 5: Detach the cluster ---"
acmlab import detach "$CLUSTER"

echo "--- Step 6: Reimport (idempotent) ---"
acmlab import cluster "$CLUSTER" \
  --kubeconfig-path "$KUBECONFIG_PATH" \
  --wait
