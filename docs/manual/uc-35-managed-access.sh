#!/bin/bash
# UC-35: ManagedServiceAccount + Cluster-Proxy for Credential-free Access
#
# Creates a ManagedServiceAccount on the spoke cluster and enables the
# cluster-proxy addon. The hub gets an auto-rotated token without storing
# static kubeconfigs or credentials.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Spoke cluster registered in ACM
#   - managed-serviceaccount addon enabled on hub
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER_NAME="spoke1"
MSA_NAME="acmlab-access"
TOKEN_TTL="720h"


# ═════════════════════════════════════════════════════════════════════
# ENABLE MANAGED ACCESS
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Enable managed-serviceaccount addon
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Enable managed-serviceaccount addon ==="

cat <<EOF | kubectl apply -f -
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: managed-serviceaccount
  namespace: ${CLUSTER_NAME}
spec:
  installNamespace: open-cluster-management-agent-addon
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 2: Enable cluster-proxy addon
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Enable cluster-proxy addon ==="

cat <<EOF | kubectl apply -f -
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: cluster-proxy
  namespace: ${CLUSTER_NAME}
spec:
  installNamespace: open-cluster-management-agent-addon
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 3: Create ManagedServiceAccount
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Create ManagedServiceAccount ==="

cat <<EOF | kubectl apply -f -
apiVersion: authentication.open-cluster-management.io/v1beta1
kind: ManagedServiceAccount
metadata:
  name: ${MSA_NAME}
  namespace: ${CLUSTER_NAME}
spec:
  rotation:
    enabled: true
    validity: ${TOKEN_TTL}
EOF


# ═════════════════════════════════════════════════════════════════════
# VERIFY
# ═════════════════════════════════════════════════════════════════════

echo "=== Check addon health ==="

kubectl get managedclusteraddon -n "$CLUSTER_NAME" \
  -o custom-columns=NAME:.metadata.name,AVAILABLE:.status.conditions[0].status

echo ""
echo "=== Check token secret ==="

kubectl get secret "$MSA_NAME" -n "$CLUSTER_NAME" 2>/dev/null && \
  echo "Token secret exists (auto-rotated every ${TOKEN_TTL})" || \
  echo "Token secret not yet created (addon still syncing)"

echo ""
echo "=== ManagedServiceAccount status ==="

kubectl get managedserviceaccount "$MSA_NAME" -n "$CLUSTER_NAME" \
  -o jsonpath='{.status.tokenSecretRef.name}'
echo ""


# ═════════════════════════════════════════════════════════════════════
# USE THE TOKEN
# ═════════════════════════════════════════════════════════════════════

# TOKEN=$(kubectl get secret ${MSA_NAME} -n ${CLUSTER_NAME} -o jsonpath='{.data.token}' | base64 -d)
# kubectl --token="$TOKEN" --server=https://cluster-proxy.example.com/${CLUSTER_NAME} get nodes


# ═════════════════════════════════════════════════════════════════════
# DISABLE
# ═════════════════════════════════════════════════════════════════════

# kubectl delete managedserviceaccount ${MSA_NAME} -n ${CLUSTER_NAME}
# kubectl delete managedclusteraddon cluster-proxy -n ${CLUSTER_NAME}
# kubectl delete managedclusteraddon managed-serviceaccount -n ${CLUSTER_NAME}


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab access enable spoke1 --ttl 720h
# acmlab access status spoke1
# acmlab access list
# acmlab access disable spoke1
