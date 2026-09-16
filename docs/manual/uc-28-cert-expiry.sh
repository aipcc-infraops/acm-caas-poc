#!/bin/bash
# UC-28: Certificate Expiry Detection Fleet-wide
#
# Deploys CertificatePolicy across the fleet to detect certificates
# nearing expiration. Non-compliant clusters are flagged in the ACM
# compliance dashboard.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - cert-policy-controller enabled on spoke clusters
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

NAMESPACE="open-cluster-management"
POLICY_NAME="cert-expiry-check"
MIN_DURATION="720h"


# ═════════════════════════════════════════════════════════════════════
# DEPLOY CERTIFICATE POLICY
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Create CertificatePolicy wrapped in Policy
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Create CertificatePolicy ==="

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
      kind: CertificatePolicy
      metadata:
        name: ${POLICY_NAME}-cert
      spec:
        severity: high
        minimumDuration: ${MIN_DURATION}
        namespaceSelector:
          include:
          - "openshift-*"
          - "kube-*"
        remediationAction: inform
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 2: Bind to all OpenShift clusters
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
# CHECK STATUS
# ═════════════════════════════════════════════════════════════════════

echo "=== Certificate compliance ==="

kubectl get policy "$POLICY_NAME" -n "$NAMESPACE" \
  -o jsonpath='{range .status.status[*]}{.clustername}: {.compliant}{"\n"}{end}'


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab policy apply cert-check --type certificate --min-duration 720h
# acmlab policy status cert-check
