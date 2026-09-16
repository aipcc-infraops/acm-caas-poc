#!/bin/bash
# UC-16: Unique Identity Provider per Cluster
#
# Deploys unique emergency htpasswd credentials to each cluster so that
# a credential leak on one cluster does not affect the rest of the fleet.
# Also enforces SSO compliance via ConfigurationPolicy.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Spoke cluster registered in ACM
#   - htpasswd utility available (httpd-tools)
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER_NAME="spoke1"
ADMIN_USER="cluster-admin"
NAMESPACE="open-cluster-management"


# ═════════════════════════════════════════════════════════════════════
# DEPLOY UNIQUE CREDENTIALS
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Generate unique password per cluster
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Generate unique password ==="

PASSWORD=$(openssl rand -base64 24)
BCRYPT_HASH=$(htpasswd -nbBC 10 "" "$PASSWORD" | tr -d ':\n' | sed 's/$2y/$2a/')
HTPASSWD_DATA=$(echo "${ADMIN_USER}:${BCRYPT_HASH}" | base64 -w0)

echo "Cluster:  $CLUSTER_NAME"
echo "User:     $ADMIN_USER"
echo "Password: $PASSWORD"
echo ""
echo "IMPORTANT: Save this password now. It is not stored on the hub."

# ─────────────────────────────────────────────────────────────────────
# Step 2: Deploy via ManifestWork
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Deploy unique IdP via ManifestWork ==="

ROTATION_TS=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

cat <<EOF | kubectl apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: idp-emergency-${CLUSTER_NAME}
  namespace: ${CLUSTER_NAME}
  annotations:
    caas/rotation-timestamp: "${ROTATION_TS}"
spec:
  workload:
    manifests:
    - apiVersion: v1
      kind: Secret
      metadata:
        name: emergency-${CLUSTER_NAME}-secret
        namespace: openshift-config
      type: Opaque
      data:
        htpasswd: ${HTPASSWD_DATA}
    - apiVersion: config.openshift.io/v1
      kind: OAuth
      metadata:
        name: cluster
      spec:
        identityProviders:
        - name: emergency-${CLUSTER_NAME}
          type: HTPasswd
          mappingMethod: claim
          htpasswd:
            fileData:
              name: emergency-${CLUSTER_NAME}-secret
EOF


# ═════════════════════════════════════════════════════════════════════
# ENFORCE SSO COMPLIANCE
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 3: Create SSO enforcement policy
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Create SSO enforcement policy ==="

cat <<EOF | kubectl apply -f -
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: sso-enforcement
  namespace: ${NAMESPACE}
spec:
  remediationAction: inform
  disabled: false
  policy-templates:
  - objectDefinition:
      apiVersion: policy.open-cluster-management.io/v1
      kind: ConfigurationPolicy
      metadata:
        name: sso-enforcement-config
      spec:
        remediationAction: inform
        severity: high
        object-templates:
        - complianceType: musthave
          objectDefinition:
            apiVersion: config.openshift.io/v1
            kind: OAuth
            metadata:
              name: cluster
            spec:
              identityProviders:
              - type: OpenID
---
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: sso-enforcement-placement
  namespace: ${NAMESPACE}
spec:
  predicates:
  - requiredClusterSelector:
      labelSelector:
        matchExpressions:
        - key: vendor
          operator: In
          values: ["OpenShift"]
---
apiVersion: policy.open-cluster-management.io/v1
kind: PlacementBinding
metadata:
  name: sso-enforcement-binding
  namespace: ${NAMESPACE}
placementRef:
  name: sso-enforcement-placement
  apiGroup: cluster.open-cluster-management.io
  kind: Placement
subjects:
- name: sso-enforcement
  apiGroup: policy.open-cluster-management.io
  kind: Policy
EOF


# ═════════════════════════════════════════════════════════════════════
# VERIFY
# ═════════════════════════════════════════════════════════════════════

echo "=== Verify deployment ==="

kubectl get manifestwork -n "$CLUSTER_NAME" "idp-emergency-${CLUSTER_NAME}" \
  -o jsonpath='{range .status.conditions[*]}{.type}: {.status}{"\n"}{end}'

echo ""
echo "SSO compliance:"
kubectl get policy sso-enforcement -n "$NAMESPACE" \
  -o jsonpath='{.status.compliant}'
echo ""


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab idp configure-unique --cluster spoke1 --admin-user cluster-admin
# acmlab idp enforce-sso
