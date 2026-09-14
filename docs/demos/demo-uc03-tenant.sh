#!/usr/bin/env bash
# UC-03: Tenant RBAC isolation via ManifestWork
# Demo: deploy tenant isolation, wait for sync, list tenants, remove
set -euo pipefail

TENANT_NAME="${1:-team-alpha}"
CLUSTER="${2:-spoke2}"
POLL_INTERVAL="${POLL_INTERVAL:-10}"
TIMEOUT="${TIMEOUT:-120}"

echo "=== UC-03: Tenant RBAC Isolation ==="

echo "--- Step 1: Deploy tenant isolation to $CLUSTER ---"
acmlab tenant deploy "$TENANT_NAME" \
  --cluster "$CLUSTER" \
  --team platform-team \
  --cpu 4 \
  --memory 8Gi

echo "--- Step 2: Wait for ManifestWork to sync ---"
elapsed=0
while true; do
  status=$(acmlab tenant status "$TENANT_NAME" 2>&1 || true)
  if echo "$status" | grep -q "Applied"; then
    echo "$status"
    break
  fi
  if [ "$elapsed" -ge "$TIMEOUT" ]; then
    echo "WARNING: ManifestWork sync timed out after ${TIMEOUT}s"
    acmlab tenant status "$TENANT_NAME"
    break
  fi
  echo "  Waiting for sync... (${elapsed}s elapsed)"
  sleep "$POLL_INTERVAL"
  elapsed=$((elapsed + POLL_INTERVAL))
done

echo "--- Step 3: List all tenants on $CLUSTER ---"
acmlab tenant list --cluster "$CLUSTER"

echo "--- Cleanup ---"
echo "To remove: acmlab tenant remove $TENANT_NAME"
