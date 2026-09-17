#!/usr/bin/env bash
# UC-46: Cluster Proxy — spoke service exposure (manual)
# Shows raw ACM resources: ManagedClusterAddOn for cluster-proxy

set -euo pipefail

CLUSTER="spoke1"

echo "=== UC-46: Cluster Proxy (manual) ==="

echo "1. Create ManagedClusterAddOn for cluster-proxy"
cat <<EOF | oc apply -f -
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: cluster-proxy
  namespace: ${CLUSTER}
  labels:
    acmlab.redhat.com/managed: "true"
spec:
  installNamespace: open-cluster-management-agent-addon
EOF

echo ""
echo "2. Verify addon created"
oc get managedclusteraddon cluster-proxy -n ${CLUSTER}

echo ""
echo "3. Check addon conditions"
oc get managedclusteraddon cluster-proxy -n ${CLUSTER} -o jsonpath='{.status.conditions[*].type}'
echo ""

echo ""
echo "=== Done ==="
