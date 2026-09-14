#!/bin/bash
# UC-03: Tenant RBAC Isolation via ManifestWork
#
# Deploy tenant isolation resources (Namespace, RoleBinding, NetworkPolicy,
# ResourceQuota) to a spoke cluster via ACM ManifestWork. The hub pushes
# the manifests; the spoke's work agent applies them.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Target cluster registered as ManagedCluster
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

TENANT_NAME="team-alpha"
CLUSTER="spoke2"
TEAM_GROUP="platform-team"
CPU_LIMIT="4"
MEMORY_LIMIT="8Gi"
POD_LIMIT="20"


# ═════════════════════════════════════════════════════════════════════
# DEPLOY TENANT ISOLATION
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Create ManifestWork with embedded resources
# ─────────────────────────────────────────────────────────────────────
# ManifestWork is created in the cluster's namespace on the hub.
# The work agent on the spoke applies all embedded manifests.

echo "=== Step 1: Create ManifestWork ==="

cat <<EOF | kubectl apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: tenant-${TENANT_NAME}
  namespace: ${CLUSTER}
spec:
  workload:
    manifests:
    - apiVersion: v1
      kind: Namespace
      metadata:
        name: ${TENANT_NAME}
        labels:
          acmlab.redhat.com/tenant: "${TENANT_NAME}"
    - apiVersion: rbac.authorization.k8s.io/v1
      kind: RoleBinding
      metadata:
        name: ${TENANT_NAME}-admin
        namespace: ${TENANT_NAME}
      roleRef:
        apiGroup: rbac.authorization.k8s.io
        kind: ClusterRole
        name: edit
      subjects:
      - apiGroup: rbac.authorization.k8s.io
        kind: Group
        name: ${TEAM_GROUP}
    - apiVersion: networking.k8s.io/v1
      kind: NetworkPolicy
      metadata:
        name: deny-cross-namespace
        namespace: ${TENANT_NAME}
      spec:
        podSelector: {}
        ingress:
        - from:
          - podSelector: {}
        policyTypes:
        - Ingress
    - apiVersion: v1
      kind: ResourceQuota
      metadata:
        name: ${TENANT_NAME}-quota
        namespace: ${TENANT_NAME}
      spec:
        hard:
          requests.cpu: "${CPU_LIMIT}"
          requests.memory: "${MEMORY_LIMIT}"
          pods: "${POD_LIMIT}"
EOF


# ═════════════════════════════════════════════════════════════════════
# CHECK SYNC STATUS
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 2: Check ManifestWork conditions
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Check ManifestWork sync ==="

echo "Conditions:"
kubectl get manifestwork "tenant-${TENANT_NAME}" -n "$CLUSTER" \
  -o jsonpath='{range .status.conditions[*]}{.type}: {.status} — {.message}{"\n"}{end}'

echo ""
echo "Per-resource status:"
kubectl get manifestwork "tenant-${TENANT_NAME}" -n "$CLUSTER" \
  -o jsonpath='{range .status.resourceStatus.manifests[*]}{.resourceMeta.kind}/{.resourceMeta.name}: {.conditions[0].type}={.conditions[0].status}{"\n"}{end}'

# ─────────────────────────────────────────────────────────────────────
# Step 3: List all tenant ManifestWorks on a cluster
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: List tenants ==="

kubectl get manifestwork -n "$CLUSTER" -l acmlab.redhat.com/tenant 2>/dev/null \
  || kubectl get manifestwork -n "$CLUSTER" --field-selector metadata.name=tenant-*


# ═════════════════════════════════════════════════════════════════════
# REMOVE
# ═════════════════════════════════════════════════════════════════════
# Deleting the ManifestWork removes all deployed resources from the spoke.

# kubectl delete manifestwork tenant-${TENANT_NAME} -n ${CLUSTER}


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab tenant deploy team-alpha --cluster spoke2 --team platform-team --cpu 4 --memory 8Gi
# acmlab tenant status team-alpha
# acmlab tenant list --cluster spoke2
# acmlab tenant remove team-alpha
