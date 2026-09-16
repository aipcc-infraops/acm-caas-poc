#!/bin/bash
# UC-25: ClusterPool and ClusterClaim — Pre-warmed Clusters
#
# Creates a Hive ClusterPool with pre-provisioned hibernated clusters.
# Developers claim clusters instantly via ClusterClaim instead of
# waiting 30+ minutes for provisioning.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Hive installed on the hub
#   - Cloud credentials configured
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

POOL_NAME="amd64-419"
POOL_NS="${POOL_NAME}"
POOL_SIZE=3
IMAGE_SET="img4.19-multi"
BASE_DOMAIN="example.com"
REGION="us-south"


# ═════════════════════════════════════════════════════════════════════
# CREATE POOL
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Create namespace
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Create namespace ==="

kubectl create namespace "$POOL_NS" --dry-run=client -o yaml | kubectl apply -f -

# ─────────────────────────────────────────────────────────────────────
# Step 2: Create ClusterPool
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Create ClusterPool ==="

cat <<EOF | kubectl apply -f -
apiVersion: hive.openshift.io/v1
kind: ClusterPool
metadata:
  name: ${POOL_NAME}
  namespace: ${POOL_NS}
spec:
  size: ${POOL_SIZE}
  baseDomain: ${BASE_DOMAIN}
  imageSetRef:
    name: ${IMAGE_SET}
  platform:
    ibmcloud:
      region: ${REGION}
      credentialsSecretRef:
        name: ${POOL_NAME}-ibmcloud-creds
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 3: Monitor pool readiness
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Pool status ==="

kubectl get clusterpool "$POOL_NAME" -n "$POOL_NS" \
  -o jsonpath='Size: {.spec.size}, Ready: {.status.ready}, Standby: {.status.standby}'
echo ""


# ═════════════════════════════════════════════════════════════════════
# CLAIM A CLUSTER
# ═════════════════════════════════════════════════════════════════════

echo "=== Claim a cluster ==="

CLAIM_NAME="my-test"

cat <<EOF | kubectl apply -f -
apiVersion: hive.openshift.io/v1
kind: ClusterClaim
metadata:
  name: ${CLAIM_NAME}
  namespace: ${POOL_NS}
spec:
  clusterPoolName: ${POOL_NAME}
  lifetime: 48h
EOF

echo "Waiting for claim to bind..."
kubectl get clusterclaim "$CLAIM_NAME" -n "$POOL_NS" \
  -o jsonpath='Cluster: {.spec.namespace}'
echo ""


# ═════════════════════════════════════════════════════════════════════
# RELEASE AND CLEANUP
# ═════════════════════════════════════════════════════════════════════

# kubectl delete clusterclaim ${CLAIM_NAME} -n ${POOL_NS}
# kubectl delete clusterpool ${POOL_NAME} -n ${POOL_NS}
# kubectl delete namespace ${POOL_NS}


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab pool create amd64-419 --size 3 --image-set img4.19-multi --platform ibmcloud
# acmlab pool list
# acmlab claim create amd64-419 --name my-test --ttl 48h
# acmlab claim release my-test
# acmlab pool delete amd64-419
