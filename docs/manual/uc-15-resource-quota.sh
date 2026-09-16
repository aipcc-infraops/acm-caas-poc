#!/bin/bash
# UC-15: Resource Quota Gates via Governance Policy
#
# Applies quota limits (max workers, max GPUs) to a cluster via labels on
# ManagedCluster, then creates a ConfigurationPolicy to monitor compliance.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Spoke cluster registered in ACM
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER_NAME="spoke1"
MAX_WORKERS=5
MAX_GPUS=1
POLICY_NAME="quota-${CLUSTER_NAME}"
NAMESPACE="open-cluster-management"


# ═════════════════════════════════════════════════════════════════════
# APPLY QUOTA
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Label ManagedCluster with quota limits
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Stamp quota labels ==="

kubectl label managedcluster "$CLUSTER_NAME" \
  "caas/max-workers=${MAX_WORKERS}" \
  "caas/max-gpus=${MAX_GPUS}" \
  --overwrite

# ─────────────────────────────────────────────────────────────────────
# Step 2: Create ConfigurationPolicy for quota enforcement
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Create quota ConfigurationPolicy ==="

cat <<EOF | kubectl apply -f -
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: ${POLICY_NAME}
  namespace: ${NAMESPACE}
spec:
  remediationAction: inform
  disabled: false
  policy-templates:
  - objectDefinition:
      apiVersion: policy.open-cluster-management.io/v1
      kind: ConfigurationPolicy
      metadata:
        name: ${POLICY_NAME}-config
      spec:
        remediationAction: inform
        severity: high
        object-templates:
        - complianceType: musthave
          objectDefinition:
            apiVersion: v1
            kind: ResourceQuota
            metadata:
              name: caas-worker-quota
              namespace: openshift-machine-api
            spec:
              hard:
                count/machines.machine.openshift.io: "${MAX_WORKERS}"
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 3: Create Placement and PlacementBinding
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Bind policy to cluster ==="

cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: ${POLICY_NAME}-placement
  namespace: ${NAMESPACE}
spec:
  predicates:
  - requiredClusterSelector:
      labelSelector:
        matchLabels:
          name: ${CLUSTER_NAME}
---
apiVersion: policy.open-cluster-management.io/v1
kind: PlacementBinding
metadata:
  name: ${POLICY_NAME}-binding
  namespace: ${NAMESPACE}
placementRef:
  name: ${POLICY_NAME}-placement
  apiGroup: cluster.open-cluster-management.io
  kind: Placement
subjects:
- name: ${POLICY_NAME}
  apiGroup: policy.open-cluster-management.io
  kind: Policy
EOF


# ═════════════════════════════════════════════════════════════════════
# CHECK STATUS
# ═════════════════════════════════════════════════════════════════════

echo "=== Check quota compliance ==="

kubectl get policy "$POLICY_NAME" -n "$NAMESPACE" \
  -o jsonpath='{.status.compliant}'
echo ""

echo "Labels on ManagedCluster:"
kubectl get managedcluster "$CLUSTER_NAME" \
  -o jsonpath='{.metadata.labels.caas/max-workers} workers, {.metadata.labels.caas/max-gpus} GPUs'
echo ""


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab policy apply-quota --cluster spoke1 --max-workers 5 --max-gpus 1
# acmlab policy quota-status spoke1
