#!/bin/bash
# UC-04: Fleet Status and Cross-Cluster Search
#
# Query fleet-wide cluster information using ManagedCluster resources.
# Read-only operations — no changes made to any cluster.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER="spoke2"


# ═════════════════════════════════════════════════════════════════════
# LIST ALL CLUSTERS
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: List ManagedClusters
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: List all clusters ==="

kubectl get managedclusters -o wide

# ─────────────────────────────────────────────────────────────────────
# Step 2: Custom columns for fleet overview
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Fleet overview ==="

kubectl get managedclusters \
  -o custom-columns=\
'NAME:.metadata.name,AVAILABLE:.status.conditions[?(@.type=="ManagedClusterConditionAvailable")].status,JOINED:.status.conditions[?(@.type=="ManagedClusterJoined")].status,VERSION:.status.version.kubernetes,CLOUD:.metadata.labels.cloud,VENDOR:.metadata.labels.vendor'


# ═════════════════════════════════════════════════════════════════════
# CLUSTER DETAILS
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 3: Detailed cluster info
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Cluster details ==="

echo "Labels:"
kubectl get managedcluster "$CLUSTER" -o jsonpath='{.metadata.labels}' | python3 -m json.tool 2>/dev/null || \
kubectl get managedcluster "$CLUSTER" -o jsonpath='{range .metadata.labels}{@}{"\n"}{end}'

echo ""
echo "Conditions:"
kubectl get managedcluster "$CLUSTER" \
  -o jsonpath='{range .status.conditions[*]}{.type}: {.status}{"\n"}{end}'

echo ""
echo "Kubernetes version:"
kubectl get managedcluster "$CLUSTER" \
  -o jsonpath='{.status.version.kubernetes}'
echo ""


# ═════════════════════════════════════════════════════════════════════
# SEARCH BY LABEL
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 4: Filter clusters by labels
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 4: Search by label ==="

echo "IBM Cloud clusters:"
kubectl get managedclusters -l cloud=IBM

echo ""
echo "OpenShift clusters:"
kubectl get managedclusters -l vendor=OpenShift

echo ""
echo "Clusters in default ClusterSet:"
kubectl get managedclusters -l 'cluster.open-cluster-management.io/clusterset=default'


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab fleet list
# acmlab fleet status spoke2
# acmlab fleet status spoke2 local-cluster    # batch
# acmlab fleet status spoke2 --json           # JSON output
