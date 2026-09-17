#!/usr/bin/env bash
# UC-26: Multi-cluster networking via Submariner
# Demonstrates enabling, status checking, and disabling Submariner

set -euo pipefail

CLUSTER_SET="${1:-prod-set}"

echo "=== UC-26: Submariner Multi-Cluster Networking ==="
echo ""

echo "Step 1: List existing Submariner deployments"
bin/acmlab submariner list
echo ""

echo "Step 2: Enable Submariner for ClusterSet ${CLUSTER_SET}"
bin/acmlab submariner enable "${CLUSTER_SET}"
echo ""

echo "Step 3: Check connectivity status"
bin/acmlab submariner status "${CLUSTER_SET}"
echo ""

echo "Step 4: List Submariner deployments (JSON)"
bin/acmlab submariner list --json
echo ""

echo "Step 5: Disable Submariner"
bin/acmlab submariner disable "${CLUSTER_SET}"
echo ""

echo "Step 6: Verify Submariner is disabled"
bin/acmlab submariner list
echo ""

echo "=== UC-26 demo complete ==="
