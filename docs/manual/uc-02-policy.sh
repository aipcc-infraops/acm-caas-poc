#!/bin/bash
# UC-02: Governance Policy Management (Image Registry Enforcement)
#
# Create a governance policy that restricts which container image registries
# clusters are allowed to pull from. Uses ACM Policy + ConfigurationPolicy
# with PlacementBinding to target clusters.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

POLICY_NAME="allowed-registries"
POLICY_NAMESPACE="open-cluster-management"
ALLOWED_REGISTRIES='["registry.redhat.io", "quay.io", "registry.access.redhat.com"]'


# ═════════════════════════════════════════════════════════════════════
# CREATE POLICY (inform mode)
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Create the Policy with ConfigurationPolicy
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Create Policy ==="

cat <<EOF | kubectl apply -f -
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: ${POLICY_NAME}
  namespace: ${POLICY_NAMESPACE}
spec:
  disabled: false
  remediationAction: inform
  policy-templates:
  - objectDefinition:
      apiVersion: policy.open-cluster-management.io/v1
      kind: ConfigurationPolicy
      metadata:
        name: ${POLICY_NAME}-config
      spec:
        remediationAction: inform
        severity: medium
        pruneObjectBehavior: None
        object-templates:
        - complianceType: musthave
          objectDefinition:
            apiVersion: config.openshift.io/v1
            kind: Image
            metadata:
              name: cluster
            spec:
              registrySources:
                allowedRegistries: ${ALLOWED_REGISTRIES}
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 2: Create PlacementRule (target all clusters)
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Create PlacementRule ==="

cat <<EOF | kubectl apply -f -
apiVersion: apps.open-cluster-management.io/v1
kind: PlacementRule
metadata:
  name: ${POLICY_NAME}-placement
  namespace: ${POLICY_NAMESPACE}
spec:
  clusterConditions:
  - status: "True"
    type: ManagedClusterConditionAvailable
  clusterSelector:
    matchExpressions: []
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 3: Create PlacementBinding
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Create PlacementBinding ==="

cat <<EOF | kubectl apply -f -
apiVersion: policy.open-cluster-management.io/v1
kind: PlacementBinding
metadata:
  name: ${POLICY_NAME}-binding
  namespace: ${POLICY_NAMESPACE}
placementRef:
  apiGroup: apps.open-cluster-management.io
  kind: PlacementRule
  name: ${POLICY_NAME}-placement
subjects:
- apiGroup: policy.open-cluster-management.io
  kind: Policy
  name: ${POLICY_NAME}
EOF


# ═════════════════════════════════════════════════════════════════════
# CHECK COMPLIANCE
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 4: Check policy compliance status
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 4: Check compliance ==="

echo "Policy compliance:"
kubectl get policy "$POLICY_NAME" -n "$POLICY_NAMESPACE" \
  -o jsonpath='{.status.compliant}'
echo ""

echo "Per-cluster status:"
kubectl get policy "$POLICY_NAME" -n "$POLICY_NAMESPACE" \
  -o jsonpath='{range .status.status[*]}{.clusterName}: {.compliant}{"\n"}{end}'


# ═════════════════════════════════════════════════════════════════════
# SWITCH TO ENFORCE
# ═════════════════════════════════════════════════════════════════════
# Enforce mode actively remediates non-compliant clusters.

# kubectl patch policy ${POLICY_NAME} -n ${POLICY_NAMESPACE} \
#   --type merge -p '{"spec":{"remediationAction":"enforce"}}'


# ═════════════════════════════════════════════════════════════════════
# REMOVE
# ═════════════════════════════════════════════════════════════════════

# kubectl delete placementbinding ${POLICY_NAME}-binding -n ${POLICY_NAMESPACE}
# kubectl delete placementrule ${POLICY_NAME}-placement -n ${POLICY_NAMESPACE}
# kubectl delete policy ${POLICY_NAME} -n ${POLICY_NAMESPACE}


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab policy apply allowed-registries \
#   --registries "registry.redhat.io,quay.io,registry.access.redhat.com" \
#   --remediation inform
#
# acmlab policy status allowed-registries
# acmlab policy apply allowed-registries --remediation enforce
# acmlab policy remove allowed-registries
