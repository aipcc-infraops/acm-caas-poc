#!/bin/bash
# UC-09: Cluster Version Upgrades via ACM
#
# Manage OCP cluster version upgrades from the hub using ManifestWork
# to patch the spoke ClusterVersion resource via ServerSideApply.
#
# Supports:
#   - Hive-provisioned clusters (ClusterDeployment exists)
#   - Imported OCP clusters (ManifestWork only)
#   - Vanilla Kubernetes (report-only — version visible but upgrades via provider)
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Cluster registered in ACM as ManagedCluster
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER="spoke2"


# ═════════════════════════════════════════════════════════════════════
# QUERY UPGRADE STATUS
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Check cluster type and distribution info
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Distribution info ==="

kubectl get managedclusterinfo "$CLUSTER" -n "$CLUSTER" \
  -o jsonpath='{.status.distributionInfo}' | python3 -m json.tool

# ─────────────────────────────────────────────────────────────────────
# Step 2: Check current OCP version and available updates
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: OCP version and updates ==="

kubectl get managedclusterinfo "$CLUSTER" -n "$CLUSTER" \
  -o jsonpath='{.status.distributionInfo.ocp.version}'
echo ""

kubectl get managedclusterinfo "$CLUSTER" -n "$CLUSTER" \
  -o jsonpath='{.status.distributionInfo.ocp.availableUpdates}'
echo ""

kubectl get managedclusterinfo "$CLUSTER" -n "$CLUSTER" \
  -o jsonpath='{.status.distributionInfo.ocp.channel}'
echo ""

# ─────────────────────────────────────────────────────────────────────
# Step 3: Check if cluster is Hive-provisioned
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Hive check ==="

kubectl get clusterdeployment "$CLUSTER" -n "$CLUSTER" -o name 2>/dev/null \
  && echo "Hive-provisioned (upgrade method: hive)" \
  || echo "Not Hive-provisioned (upgrade method: manifestwork or report-only)"

# ─────────────────────────────────────────────────────────────────────
# Step 4: Check version upgrade history
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 4: Version history ==="

kubectl get managedclusterinfo "$CLUSTER" -n "$CLUSTER" \
  -o jsonpath='{.status.distributionInfo.ocp.versionHistory}' | python3 -m json.tool


# ═════════════════════════════════════════════════════════════════════
# SET UPDATE CHANNEL
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 5: Create ManifestWork to set channel on spoke ClusterVersion
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 5: Set channel via ManifestWork ==="

CHANNEL="stable-4.22"

cat <<EOF | kubectl apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: ${CLUSTER}-channel
  namespace: ${CLUSTER}
  labels:
    caas-poc/operation: upgrade
    caas-poc/cluster: ${CLUSTER}
spec:
  workload:
    manifests:
      - apiVersion: config.openshift.io/v1
        kind: ClusterVersion
        metadata:
          name: version
        spec:
          channel: ${CHANNEL}
  manifestConfigs:
    - resourceIdentifier:
        group: config.openshift.io
        resource: clusterversions
        name: version
        namespace: ""
      updateStrategy:
        type: ServerSideApply
EOF


# ═════════════════════════════════════════════════════════════════════
# START UPGRADE
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 6: Create ManifestWork to trigger version upgrade
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 6: Start upgrade via ManifestWork ==="

TARGET_VERSION="4.22.10"

cat <<EOF | kubectl apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: ${CLUSTER}-upgrade
  namespace: ${CLUSTER}
  labels:
    caas-poc/operation: upgrade
    caas-poc/cluster: ${CLUSTER}
spec:
  workload:
    manifests:
      - apiVersion: config.openshift.io/v1
        kind: ClusterVersion
        metadata:
          name: version
        spec:
          desiredUpdate:
            version: ${TARGET_VERSION}
  manifestConfigs:
    - resourceIdentifier:
        group: config.openshift.io
        resource: clusterversions
        name: version
        namespace: ""
      updateStrategy:
        type: ServerSideApply
EOF


# ═════════════════════════════════════════════════════════════════════
# MONITOR PROGRESS
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 7: Watch ManifestWork status
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 7: ManifestWork status ==="

kubectl get manifestwork "${CLUSTER}-upgrade" -n "$CLUSTER" \
  -o jsonpath='{.status.conditions}' | python3 -m json.tool

# ─────────────────────────────────────────────────────────────────────
# Step 8: Watch ManagedClusterInfo for version change
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 8: Monitor version progress ==="

kubectl get managedclusterinfo "$CLUSTER" -n "$CLUSTER" \
  -o jsonpath='version={.status.distributionInfo.ocp.version} desired={.status.distributionInfo.ocp.desiredVersion}'
echo ""
