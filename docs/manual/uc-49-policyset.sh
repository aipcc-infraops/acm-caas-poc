#!/usr/bin/env bash
# UC-49: PolicySet compliance profiles (manual)
# Shows raw ACM resources: PolicySet, Placement, PlacementBinding

set -euo pipefail

NAME="cis-golden-config"
NAMESPACE="global-set"

echo "=== UC-49: PolicySet (manual) ==="

echo "1. Create PolicySet grouping multiple policies"
cat <<EOF | oc apply -f -
apiVersion: policy.open-cluster-management.io/v1beta1
kind: PolicySet
metadata:
  name: ${NAME}
  namespace: ${NAMESPACE}
  labels:
    acmlab.redhat.com/managed: "true"
spec:
  description: "CIS golden configuration"
  policies:
    - cert-expiry
    - image-registry
    - operator-pin
EOF

echo ""
echo "2. Create Placement for PolicySet"
cat <<EOF | oc apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: ${NAME}-placement
  namespace: ${NAMESPACE}
  labels:
    acmlab.redhat.com/managed: "true"
spec:
  predicates:
    - requiredClusterSelector:
        labelSelector: {}
EOF

echo ""
echo "3. Create PlacementBinding"
cat <<EOF | oc apply -f -
apiVersion: policy.open-cluster-management.io/v1
kind: PlacementBinding
metadata:
  name: ${NAME}-binding
  namespace: ${NAMESPACE}
  labels:
    acmlab.redhat.com/managed: "true"
placementRef:
  apiGroup: cluster.open-cluster-management.io
  kind: Placement
  name: ${NAME}-placement
subjects:
  - apiGroup: policy.open-cluster-management.io
    kind: PolicySet
    name: ${NAME}
EOF

echo ""
echo "4. Verify PolicySet status"
oc get policyset ${NAME} -n ${NAMESPACE}

echo ""
echo "=== Done ==="
