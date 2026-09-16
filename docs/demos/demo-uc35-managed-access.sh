#!/usr/bin/env bash
# UC-35: ManagedServiceAccount + cluster-proxy — credential-free access
# Demo: enable, check status, list, disable
set -euo pipefail

CLUSTER="${1:-spoke2}"

echo "=== UC-35: Credential-Free Hub-to-Spoke Access ==="

echo "--- Step 1: Enable managed access ---"
acmlab access enable "$CLUSTER" --ttl 720h --roles cluster-admin

echo "--- Step 2: Check access status ---"
acmlab access status "$CLUSTER"

echo "--- Step 3: List all managed access ---"
acmlab access list

echo "--- Step 4: Disable managed access ---"
acmlab access disable "$CLUSTER"

echo ""
echo "=== Demo complete ==="
echo "Cluster $CLUSTER had auto-rotated token access enabled and then disabled."
echo "No static kubeconfig was needed at any point."
