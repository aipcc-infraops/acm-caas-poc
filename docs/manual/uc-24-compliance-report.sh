#!/bin/bash
# UC-24: Per-Team Compliance Reporting
#
# Generates compliance reports scoped by ClusterSet, so each team sees
# only their clusters' policy compliance status.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - ClusterSets configured (UC-22)
#   - Policies applied to clusters
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

SET_NAME="team-gpu"
NAMESPACE="open-cluster-management"


# ═════════════════════════════════════════════════════════════════════
# COMPLIANCE REPORT
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Get clusters in the ClusterSet
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Clusters in ${SET_NAME} ==="

CLUSTERS=$(kubectl get managedcluster \
  -l "cluster.open-cluster-management.io/clusterset=${SET_NAME}" \
  -o jsonpath='{.items[*].metadata.name}')

echo "Clusters: $CLUSTERS"

# ─────────────────────────────────────────────────────────────────────
# Step 2: Get policy compliance for each cluster
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Policy compliance by cluster ==="

for CLUSTER in $CLUSTERS; do
  echo ""
  echo "--- $CLUSTER ---"
  kubectl get policy -n "$NAMESPACE" -o json | \
    jq -r --arg c "$CLUSTER" '.items[] |
      select(.status.status[]? | select(.clustername == $c)) |
      "\(.metadata.name): \(.status.status[] | select(.clustername == $c) | .compliant)"'
done

# ─────────────────────────────────────────────────────────────────────
# Step 3: Summary counts
# ─────────────────────────────────────────────────────────────────────
echo ""
echo "=== Step 3: Compliance summary ==="

for CLUSTER in $CLUSTERS; do
  COMPLIANT=$(kubectl get policy -n "$NAMESPACE" -o json | \
    jq --arg c "$CLUSTER" '[.items[].status.status[]? | select(.clustername == $c and .compliant == "Compliant")] | length')
  NONCOMPLIANT=$(kubectl get policy -n "$NAMESPACE" -o json | \
    jq --arg c "$CLUSTER" '[.items[].status.status[]? | select(.clustername == $c and .compliant == "NonCompliant")] | length')
  echo "$CLUSTER: ${COMPLIANT} compliant, ${NONCOMPLIANT} non-compliant"
done


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab policy report --cluster-set team-gpu
