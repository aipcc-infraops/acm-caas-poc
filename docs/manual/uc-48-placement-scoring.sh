#!/usr/bin/env bash
# UC-48: Placement scoring — resource-based scheduling (manual)
# Shows raw ACM resources: Placement with prioritizers, AddOnPlacementScore

set -euo pipefail

NAME="gpu-scoring"
NAMESPACE="open-cluster-management"

echo "=== UC-48: Placement Scoring (manual) ==="

echo "1. Create Placement with resource prioritizers"
cat <<EOF | oc apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: ${NAME}
  namespace: ${NAMESPACE}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/scoring: "true"
spec:
  prioritizerPolicy:
    mode: Exact
    configurations:
      - scoreCoordinate:
          type: BuiltIn
          builtIn: ResourceAllocatableCPU
        weight: 1
      - scoreCoordinate:
          type: BuiltIn
          builtIn: ResourceAllocatableMemory
        weight: 1
EOF

echo ""
echo "2. Check placement decisions"
oc get placementdecisions -n ${NAMESPACE} -l "cluster.open-cluster-management.io/placement=${NAME}"

echo ""
echo "3. View AddOnPlacementScores"
oc get addonplacementscores --all-namespaces

echo ""
echo "4. Delete scoring placement"
oc delete placement ${NAME} -n ${NAMESPACE}

echo ""
echo "=== Done ==="
