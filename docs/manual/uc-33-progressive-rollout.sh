#!/bin/bash
# UC-33: ManifestWorkReplicaSet Progressive Rollout
#
# Creates a ManifestWorkReplicaSet that fans out manifests to clusters
# selected by a Placement, with progressive rollout strategy to limit
# blast radius during fleet-wide updates.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Spoke clusters registered in ACM
#   - Placement resource targeting desired clusters
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

NAME="kueue-v12"
NAMESPACE="open-cluster-management"
PLACEMENT="gpu-clusters"
MAX_CONCURRENCY=2
MAX_FAILURES="10%"


# ═════════════════════════════════════════════════════════════════════
# CREATE PLACEMENT
# ═════════════════════════════════════════════════════════════════════

echo "=== Step 1: Create Placement for target clusters ==="

cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: ${PLACEMENT}
  namespace: ${NAMESPACE}
spec:
  predicates:
  - requiredClusterSelector:
      labelSelector:
        matchLabels:
          caas/gpu: "true"
EOF


# ═════════════════════════════════════════════════════════════════════
# CREATE MANIFESTWORKREPLICASET
# ═════════════════════════════════════════════════════════════════════

echo "=== Step 2: Create ManifestWorkReplicaSet with progressive rollout ==="

cat <<EOF | kubectl apply -f -
apiVersion: work.open-cluster-management.io/v1alpha1
kind: ManifestWorkReplicaSet
metadata:
  name: ${NAME}
  namespace: ${NAMESPACE}
spec:
  placementRefs:
  - name: ${PLACEMENT}
    rolloutStrategy:
      rolloutType: Progressive
      progressive:
        maxConcurrency: ${MAX_CONCURRENCY}
        maxFailures: "${MAX_FAILURES}"
        progressDeadline: "10m"
  manifestWorkTemplate:
    workload:
      manifests:
      - apiVersion: v1
        kind: Namespace
        metadata:
          name: kueue-system
      - apiVersion: v1
        kind: ConfigMap
        metadata:
          name: kueue-config
          namespace: kueue-system
        data:
          version: "1.2"
EOF


# ═════════════════════════════════════════════════════════════════════
# MONITOR ROLLOUT
# ═════════════════════════════════════════════════════════════════════

echo "=== Step 3: Check rollout status ==="

kubectl get manifestworkreplicaset "$NAME" -n "$NAMESPACE" \
  -o jsonpath='Strategy: {.spec.placementRefs[0].rolloutStrategy.rolloutType}'
echo ""

kubectl get manifestworkreplicaset "$NAME" -n "$NAMESPACE" \
  -o jsonpath='Summary: Applied={.status.summary.applied}, Total={.status.summary.total}, Failed={.status.summary.failed}'
echo ""


# ═════════════════════════════════════════════════════════════════════
# UPDATE STRATEGY
# ═════════════════════════════════════════════════════════════════════

# To change strategy mid-rollout:
# kubectl patch manifestworkreplicaset ${NAME} -n ${NAMESPACE} --type merge -p '
# {
#   "spec": {
#     "placementRefs": [{
#       "name": "'${PLACEMENT}'",
#       "rolloutStrategy": {
#         "rolloutType": "All"
#       }
#     }]
#   }
# }'


# ═════════════════════════════════════════════════════════════════════
# CLEANUP
# ═════════════════════════════════════════════════════════════════════

# kubectl delete manifestworkreplicaset ${NAME} -n ${NAMESPACE}
# kubectl delete placement ${PLACEMENT} -n ${NAMESPACE}


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab rollout create kueue-v12 --placement gpu-clusters --strategy progressive --max-concurrency 2 --max-failures 10%
# acmlab rollout get kueue-v12
# acmlab rollout update-strategy kueue-v12 --strategy all
# acmlab rollout delete kueue-v12
