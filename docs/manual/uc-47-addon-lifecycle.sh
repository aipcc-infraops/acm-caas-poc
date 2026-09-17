#!/usr/bin/env bash
# UC-47: Add-on lifecycle management (manual)
# Shows raw ACM resources: AddOnDeploymentConfig

set -euo pipefail

CONFIG_NAME="observability-config"
NAMESPACE="open-cluster-management"

echo "=== UC-47: Add-on Lifecycle (manual) ==="

echo "1. List ClusterManagementAddOns"
oc get clustermanagementaddons

echo ""
echo "2. Create AddOnDeploymentConfig"
cat <<EOF | oc apply -f -
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: AddOnDeploymentConfig
metadata:
  name: ${CONFIG_NAME}
  namespace: ${NAMESPACE}
  labels:
    acmlab.redhat.com/managed: "true"
spec:
  customizedVariables:
    - name: replica-count
      value: "3"
    - name: log-level
      value: debug
EOF

echo ""
echo "3. Verify config created"
oc get addondeploymentconfig ${CONFIG_NAME} -n ${NAMESPACE}

echo ""
echo "4. Delete config"
oc delete addondeploymentconfig ${CONFIG_NAME} -n ${NAMESPACE}

echo ""
echo "=== Done ==="
