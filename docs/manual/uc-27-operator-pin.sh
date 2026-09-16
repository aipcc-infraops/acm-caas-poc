#!/bin/bash
# UC-27: Operator Version Pinning via OperatorPolicy
#
# Pins an OLM-managed operator to a specific version across the fleet
# using ACM's OperatorPolicy. Prevents automatic upgrades that could
# break workloads.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Spoke clusters registered in ACM
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

OPERATOR_NAME="gpu-operator"
CHANNEL="v24.3"
VERSION="24.3.0"
NAMESPACE="open-cluster-management"
POLICY_NAME="pin-${OPERATOR_NAME}"


# ═════════════════════════════════════════════════════════════════════
# PIN OPERATOR VERSION
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Create OperatorPolicy
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Create OperatorPolicy ==="

cat <<EOF | kubectl apply -f -
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: ${POLICY_NAME}
  namespace: ${NAMESPACE}
spec:
  remediationAction: enforce
  disabled: false
  policy-templates:
  - objectDefinition:
      apiVersion: policy.open-cluster-management.io/v1beta1
      kind: OperatorPolicy
      metadata:
        name: ${POLICY_NAME}-operator
      spec:
        remediationAction: enforce
        severity: high
        complianceType: musthave
        subscription:
          channel: ${CHANNEL}
          name: ${OPERATOR_NAME}
          namespace: openshift-operators
          source: certified-operators
          sourceNamespace: openshift-marketplace
          startingCSV: ${OPERATOR_NAME}.v${VERSION}
          installPlanApproval: Manual
        upgradeApproval: None
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 2: Bind to clusters
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Create Placement and binding ==="

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
          vendor: OpenShift
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
# VERIFY
# ═════════════════════════════════════════════════════════════════════

echo "=== Check compliance ==="

kubectl get policy "$POLICY_NAME" -n "$NAMESPACE" \
  -o jsonpath='{.status.compliant}'
echo ""


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab policy apply my-operator-pin --type operator --operator gpu-operator --channel v24.3 --version 24.3.0
# acmlab policy status my-operator-pin
