#!/usr/bin/env bash
# UC-26: Multi-cluster networking via Submariner (manual)
# Uses raw oc/kubectl commands to configure Submariner

set -euo pipefail

CLUSTER_SET="${1:-prod-set}"
CLUSTER_1="${2:-spoke1}"
CLUSTER_2="${3:-spoke2}"

echo "=== UC-26: Submariner Multi-Cluster Networking (manual) ==="
echo ""

echo "Step 1: Create ManagedClusterAddOn for ${CLUSTER_1}"
oc apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: submariner
  namespace: ${CLUSTER_1}
  labels:
    acmlab.redhat.com/managed: "true"
spec:
  installNamespace: submariner-operator
EOF
echo ""

echo "Step 2: Create SubmarinerConfig for ${CLUSTER_1}"
oc apply -f - <<EOF
apiVersion: submarineraddon.open-cluster-management.io/v1alpha1
kind: SubmarinerConfig
metadata:
  name: submariner
  namespace: ${CLUSTER_1}
  labels:
    acmlab.redhat.com/managed: "true"
spec:
  IPSecNATTPort: 4500
  NATTEnable: true
  cableDriver: libreswan
  gatewayConfig:
    gateways: 1
  credentialsSecret:
    name: ${CLUSTER_1}-submariner-creds
EOF
echo ""

echo "Step 3: Label cluster as submariner-enabled"
oc label managedcluster "${CLUSTER_1}" submariner=enabled --overwrite
echo ""

echo "Step 4: Repeat for ${CLUSTER_2}"
oc apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: submariner
  namespace: ${CLUSTER_2}
  labels:
    acmlab.redhat.com/managed: "true"
spec:
  installNamespace: submariner-operator
EOF

oc apply -f - <<EOF
apiVersion: submarineraddon.open-cluster-management.io/v1alpha1
kind: SubmarinerConfig
metadata:
  name: submariner
  namespace: ${CLUSTER_2}
  labels:
    acmlab.redhat.com/managed: "true"
spec:
  IPSecNATTPort: 4500
  NATTEnable: true
  cableDriver: libreswan
  gatewayConfig:
    gateways: 1
  credentialsSecret:
    name: ${CLUSTER_2}-submariner-creds
EOF

oc label managedcluster "${CLUSTER_2}" submariner=enabled --overwrite
echo ""

echo "Step 5: Check Submariner AddOn status"
oc get managedclusteraddons -n "${CLUSTER_1}" submariner -o yaml
oc get managedclusteraddons -n "${CLUSTER_2}" submariner -o yaml
echo ""

echo "Step 6: Verify connectivity"
oc get managedclusters -l submariner=enabled -o custom-columns=NAME:.metadata.name,SET:.metadata.labels.cluster\\.open-cluster-management\\.io/clusterset
echo ""

echo "Step 7: Cleanup (disable Submariner)"
oc delete managedclusteraddon -n "${CLUSTER_1}" submariner --ignore-not-found
oc delete managedclusteraddon -n "${CLUSTER_2}" submariner --ignore-not-found
oc delete submarinerconfig -n "${CLUSTER_1}" submariner --ignore-not-found
oc delete submarinerconfig -n "${CLUSTER_2}" submariner --ignore-not-found
oc label managedcluster "${CLUSTER_1}" submariner- --overwrite
oc label managedcluster "${CLUSTER_2}" submariner- --overwrite
echo ""

echo "=== UC-26 manual complete ==="
