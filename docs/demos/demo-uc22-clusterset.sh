#!/usr/bin/env bash
# UC-22: ClusterSet management — team isolation and multi-tenancy
# Demo: create a ClusterSet, assign clusters, list, clean up
set -euo pipefail

TEAM="${1:-team-serving}"
NAMESPACE="${2:-serving-ns}"
CLUSTER="${3:-spoke2}"

echo "=== UC-22: ClusterSet Management ==="

echo "--- Step 1: Create ClusterSet with binding ---"
acmlab clusterset create "$TEAM" --namespace "$NAMESPACE"

echo "--- Step 2: List ClusterSets ---"
acmlab clusterset list

echo "--- Step 3: Assign cluster $CLUSTER to $TEAM ---"
acmlab clusterset assign "$CLUSTER" --to "$TEAM"

echo "--- Step 4: Verify membership ---"
acmlab clusterset list

echo "--- Step 5: Clean up ---"
echo "  acmlab clusterset remove $TEAM --namespace $NAMESPACE"

echo "=== Demo complete ==="
