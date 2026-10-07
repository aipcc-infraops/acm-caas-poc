#!/usr/bin/env bash
# UC-26: Multi-cluster networking via Submariner
# Demonstrates enabling, status checking, diagnosing, and testing Submariner

set -euo pipefail

CLUSTER_SET="${1:-prod-set}"

echo "=== UC-26: Submariner Multi-Cluster Networking ==="
echo ""

echo "Step 1: List existing Submariner deployments"
bin/acmlab submariner list
echo ""

echo "Step 2: Create a dedicated test ClusterSet (if needed)"
echo "  bin/acmlab submariner create-test-set uc26-test --clusters caas-pool-1,caas-pool-2 --confirm"
echo ""

echo "Step 3: Enable Submariner for ClusterSet ${CLUSTER_SET}"
bin/acmlab submariner enable "${CLUSTER_SET}"
echo ""

echo "Step 3b: Enable with Globalnet (for overlapping CIDRs)"
echo "  bin/acmlab submariner enable ${CLUSTER_SET} --globalnet"
echo ""

echo "Step 3c: Enable with --wait (blocks until connected)"
echo "  bin/acmlab submariner enable ${CLUSTER_SET} --wait --timeout 10m"
echo ""

echo "Step 4: Check connectivity status"
bin/acmlab submariner status "${CLUSTER_SET}"
echo ""

echo "Step 5: Diagnose connectivity issues"
bin/acmlab submariner diagnose "${CLUSTER_SET}"
echo ""

echo "Step 6: Test cross-cluster connectivity (requires spoke kubeconfigs)"
echo "  bin/acmlab submariner test-connectivity spoke1 spoke2 \\"
echo "    --kubeconfig-a /path/to/spoke1.kubeconfig \\"
echo "    --kubeconfig-b /path/to/spoke2.kubeconfig \\"
echo "    --timeout 2m"
echo ""

echo "Step 7: List Submariner deployments (JSON)"
bin/acmlab submariner list --json
echo ""

echo "Step 8: Disable Submariner"
bin/acmlab submariner disable "${CLUSTER_SET}"
echo ""

echo "Step 9: Verify Submariner is disabled"
bin/acmlab submariner list
echo ""

echo "=== UC-26 demo complete ==="
