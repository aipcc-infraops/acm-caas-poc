#!/usr/bin/env bash
# UC-09: Cluster version upgrades (Day-2 OCP)
# Demo: check status, list upgradeable, set channel, start upgrade, check history
set -euo pipefail

CLUSTER="${1:-spoke2}"

echo "=== UC-09: Cluster Version Upgrades ==="

echo "--- Step 1: List clusters with available upgrades ---"
acmlab upgrade list

echo "--- Step 2: Get upgrade status for $CLUSTER ---"
acmlab upgrade status "$CLUSTER"

echo "--- Step 3: Get version upgrade history ---"
acmlab upgrade history "$CLUSTER"

echo "--- Step 4: Set channel (example: fast-4.22) ---"
echo "  acmlab upgrade set-channel $CLUSTER fast-4.22"
echo "  (skipped in demo — run manually to change channel)"

echo "--- Step 5: Start upgrade (example: 4.22.10) ---"
echo "  acmlab upgrade start $CLUSTER 4.22.10"
echo "  (skipped in demo — run manually to trigger upgrade)"

echo "--- Step 6: JSON output ---"
acmlab upgrade status "$CLUSTER" --json

echo ""
echo "--- Upgrade methods ---"
echo "  hive:         Hive-provisioned OCP clusters (ManifestWork patches ClusterVersion)"
echo "  manifestwork: Imported OCP clusters (ManifestWork patches ClusterVersion)"
echo "  report-only:  Vanilla Kubernetes clusters (version displayed, upgrades via provider tools)"
