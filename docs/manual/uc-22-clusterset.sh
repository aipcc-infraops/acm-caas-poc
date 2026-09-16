#!/bin/bash
# UC-22: ClusterSet Management — Team Isolation
#
# Creates ManagedClusterSets for team isolation and binds them to
# namespaces for scoped policy/placement access.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Spoke clusters registered in ACM
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

SET_NAME="team-gpu"
NAMESPACE="team-gpu-ns"
CLUSTER_NAME="spoke1"


# ═════════════════════════════════════════════════════════════════════
# CREATE CLUSTERSET
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Create ManagedClusterSet
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Create ManagedClusterSet ==="

cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta2
kind: ManagedClusterSet
metadata:
  name: ${SET_NAME}
spec:
  clusterSelector:
    selectorType: ExclusiveClusterSetLabel
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 2: Assign cluster to set
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Label cluster into set ==="

kubectl label managedcluster "$CLUSTER_NAME" \
  "cluster.open-cluster-management.io/clusterset=${SET_NAME}" \
  --overwrite

# ─────────────────────────────────────────────────────────────────────
# Step 3: Create namespace and binding
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Create namespace and binding ==="

kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta2
kind: ManagedClusterSetBinding
metadata:
  name: ${SET_NAME}
  namespace: ${NAMESPACE}
spec:
  clusterSet: ${SET_NAME}
EOF


# ═════════════════════════════════════════════════════════════════════
# VERIFY
# ═════════════════════════════════════════════════════════════════════

echo "=== List ClusterSets ==="
kubectl get managedclusterset

echo ""
echo "=== Clusters in set ==="
kubectl get managedcluster -l "cluster.open-cluster-management.io/clusterset=${SET_NAME}"


# ═════════════════════════════════════════════════════════════════════
# REMOVE
# ═════════════════════════════════════════════════════════════════════

# kubectl label managedcluster ${CLUSTER_NAME} cluster.open-cluster-management.io/clusterset-
# kubectl delete managedclustersetbinding ${SET_NAME} -n ${NAMESPACE}
# kubectl delete managedclusterset ${SET_NAME}


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab clusterset create team-gpu
# acmlab clusterset assign spoke1 --set team-gpu
# acmlab clusterset list
# acmlab clusterset remove team-gpu
