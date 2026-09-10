#!/bin/bash
# UC-07: External Cluster Import and Detach
#
# Import an existing cluster into ACM management, or detach it.
# Two modes: auto-import (provide kubeconfig) or manual (apply yamls on spoke).
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - For auto-import: kubeconfig for the spoke cluster
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER_NAME="my-external-cluster"
SPOKE_KUBECONFIG="/tmp/spoke.kubeconfig"  # for auto-import

# ═════════════════════════════════════════════════════════════════════
# AUTO-IMPORT (recommended)
# ═════════════════════════════════════════════════════════════════════
# ACM reads the kubeconfig from a secret and installs the klusterlet
# on the spoke automatically. No manual steps on the spoke.

# ─────────────────────────────────────────────────────────────────────
# Step 1: Create namespace for the cluster
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Create namespace ==="

kubectl create namespace "$CLUSTER_NAME" --dry-run=client -o yaml | kubectl apply -f -

# ─────────────────────────────────────────────────────────────────────
# Step 2: Create ManagedCluster
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Create ManagedCluster ==="

cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: ${CLUSTER_NAME}
  labels:
    name: ${CLUSTER_NAME}
    cloud: auto-detect
    vendor: auto-detect
    cluster.open-cluster-management.io/clusterset: default
spec:
  hubAcceptsClient: true
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 3: Create KlusterletAddonConfig
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Create KlusterletAddonConfig ==="

cat <<EOF | kubectl apply -f -
apiVersion: agent.open-cluster-management.io/v1
kind: KlusterletAddonConfig
metadata:
  name: ${CLUSTER_NAME}
  namespace: ${CLUSTER_NAME}
spec:
  applicationManager:
    enabled: true
  certPolicyController:
    enabled: true
  policyController:
    enabled: true
  searchCollector:
    enabled: true
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 4: Create auto-import secret
# ─────────────────────────────────────────────────────────────────────
# ACM's import controller watches for this secret, connects to the spoke,
# and applies the klusterlet manifests. Then it deletes the secret.

echo "=== Step 4: Create auto-import secret ==="

kubectl create secret generic auto-import-secret \
  --namespace "$CLUSTER_NAME" \
  --from-file=kubeconfig="$SPOKE_KUBECONFIG" \
  --dry-run=client -o yaml | kubectl apply -f -

# ─────────────────────────────────────────────────────────────────────
# Step 5: Wait for import
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 5: Wait for import ==="

echo "Waiting for ManagedCluster to become available..."
kubectl wait managedcluster "$CLUSTER_NAME" \
  --for=condition=ManagedClusterConditionAvailable \
  --timeout=10m

echo "Cluster $CLUSTER_NAME successfully imported"

# ─────────────────────────────────────────────────────────────────────
# Check status
# ─────────────────────────────────────────────────────────────────────
echo ""
echo "=== Status ==="

kubectl get managedcluster "$CLUSTER_NAME" -o jsonpath='{range .status.conditions[*]}{.type}: {.status}{"\n"}{end}'


# ═════════════════════════════════════════════════════════════════════
# MANUAL IMPORT
# ═════════════════════════════════════════════════════════════════════
# If you don't have a kubeconfig for the spoke, skip step 4 above.
# ACM generates import manifests that you apply manually on the spoke.
#
# After steps 1-3, run:
#
# # Get the import secret (contains CRDs and import manifests)
# kubectl get secret ${CLUSTER_NAME}-import -n ${CLUSTER_NAME} -o jsonpath='{.data.crds\.yaml}' | base64 -d > /tmp/crds.yaml
# kubectl get secret ${CLUSTER_NAME}-import -n ${CLUSTER_NAME} -o jsonpath='{.data.import\.yaml}' | base64 -d > /tmp/import.yaml
#
# # Apply on the spoke cluster
# KUBECONFIG=/path/to/spoke kubectl apply -f /tmp/crds.yaml
# KUBECONFIG=/path/to/spoke kubectl apply -f /tmp/import.yaml


# ═════════════════════════════════════════════════════════════════════
# DETACH
# ═════════════════════════════════════════════════════════════════════
# Detaching removes the cluster from ACM management but does NOT destroy
# the underlying cluster. ACM's cleanup controllers remove the klusterlet
# from the spoke.

# kubectl delete managedcluster ${CLUSTER_NAME}
#
# Wait for cleanup:
# kubectl wait managedcluster ${CLUSTER_NAME} --for=delete --timeout=5m


# ═════════════════════════════════════════════════════════════════════
# REIMPORT (after detach)
# ═════════════════════════════════════════════════════════════════════
# After detaching, re-run steps 1-4 (or 1-3 for manual).
# The namespace may still exist — that's fine, step 1 uses --dry-run.


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# # Auto-import with kubeconfig file
# acmlab import cluster my-external-cluster --kubeconfig-path /tmp/spoke.kubeconfig --wait
#
# # Auto-import using a context from default kubeconfig
# acmlab import cluster my-external-cluster --kubeconfig-context spoke-context --wait
#
# # Manual import (no kubeconfig flags)
# acmlab import cluster my-external-cluster
#
# # Status
# acmlab import status my-external-cluster
#
# # List imported clusters
# acmlab import list
#
# # Detach
# acmlab import detach my-external-cluster
