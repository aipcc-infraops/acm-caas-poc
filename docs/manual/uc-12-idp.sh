#!/bin/bash
# UC-12: Identity Provider Management via ManifestWork
#
# Configures an Identity Provider (htpasswd, GitHub, Google, OIDC) on a spoke
# cluster by deploying an OAuth CR patch through ManifestWork.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Spoke cluster registered in ACM
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER_NAME="spoke1"
IDP_NAME="lab-htpasswd"
ADMIN_USER="cluster-admin"
ADMIN_PASSWORD="$(openssl rand -base64 24)"


# ═════════════════════════════════════════════════════════════════════
# CONFIGURE HTPASSWD IdP
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Generate bcrypt hash
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Generate bcrypt password hash ==="

BCRYPT_HASH=$(htpasswd -nbBC 10 "" "$ADMIN_PASSWORD" | tr -d ':\n' | sed 's/$2y/$2a/')
echo "Password: $ADMIN_PASSWORD"
echo "Hash: $BCRYPT_HASH"

# ─────────────────────────────────────────────────────────────────────
# Step 2: Create ManifestWork with htpasswd Secret + OAuth CR
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Deploy IdP via ManifestWork ==="

HTPASSWD_DATA=$(echo "${ADMIN_USER}:${BCRYPT_HASH}" | base64 -w0)

cat <<EOF | kubectl apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: idp-${IDP_NAME}
  namespace: ${CLUSTER_NAME}
spec:
  workload:
    manifests:
    - apiVersion: v1
      kind: Secret
      metadata:
        name: ${IDP_NAME}-secret
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
        - name: ${IDP_NAME}
          type: HTPasswd
          mappingMethod: claim
          htpasswd:
            fileData:
              name: ${IDP_NAME}-secret
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 3: Verify deployment
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Verify ManifestWork status ==="

kubectl get manifestwork -n "$CLUSTER_NAME" "idp-${IDP_NAME}" \
  -o jsonpath='{range .status.conditions[*]}{.type}: {.status}{"\n"}{end}'

# ─────────────────────────────────────────────────────────────────────
# Step 4: List IdPs on cluster
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 4: List IdP ManifestWorks ==="

kubectl get manifestwork -n "$CLUSTER_NAME" -l 'app.kubernetes.io/component=idp'


# ═════════════════════════════════════════════════════════════════════
# REMOVE
# ═════════════════════════════════════════════════════════════════════

# kubectl delete manifestwork idp-${IDP_NAME} -n ${CLUSTER_NAME}


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab idp configure lab-htpasswd --cluster spoke1 --type htpasswd --users admin=secret123
# acmlab idp list --cluster spoke1
# acmlab idp rotate lab-htpasswd --cluster spoke1
# acmlab idp remove lab-htpasswd --cluster spoke1
