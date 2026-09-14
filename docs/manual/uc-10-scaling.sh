#!/bin/bash
# UC-10: Cluster Scaling via Hive MachinePool
#
# Scale worker nodes on Hive-provisioned clusters by managing MachinePool
# resources. Supports fixed replica counts and autoscaling.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Cluster provisioned by Hive (has a ClusterDeployment)
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER="spoke2"
WORKER_TYPE="bx2-4x16"
PLATFORM="ibmcloud"


# ═════════════════════════════════════════════════════════════════════
# LIST MACHINEPOOLS
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: List all MachinePools across the fleet
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: List MachinePools ==="

kubectl get machinepools.hive.openshift.io -A \
  -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,REPLICAS:.spec.replicas,MIN:.spec.autoscaling.minReplicas,MAX:.spec.autoscaling.maxReplicas'

# ─────────────────────────────────────────────────────────────────────
# Step 2: Get details for a specific cluster
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: MachinePool details ==="

kubectl get machinepool.hive.openshift.io -n "$CLUSTER" -o yaml


# ═════════════════════════════════════════════════════════════════════
# SCALE WORKERS (fixed replicas)
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 3: Set fixed replica count
# ─────────────────────────────────────────────────────────────────────
# Patching replicas removes autoscaling if active.

echo "=== Step 3: Scale to 3 workers ==="

POOL_NAME=$(kubectl get machinepool.hive.openshift.io -n "$CLUSTER" -o jsonpath='{.items[0].metadata.name}')

kubectl patch machinepool.hive.openshift.io "$POOL_NAME" -n "$CLUSTER" \
  --type merge -p '{"spec":{"replicas":3,"autoscaling":null}}'

echo "Scaled $POOL_NAME to 3 replicas"


# ═════════════════════════════════════════════════════════════════════
# ENABLE AUTOSCALING
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 4: Enable autoscaling with min/max bounds
# ─────────────────────────────────────────────────────────────────────
# Setting autoscaling removes the fixed replicas field.

echo "=== Step 4: Enable autoscaling (min=2, max=5) ==="

kubectl patch machinepool.hive.openshift.io "$POOL_NAME" -n "$CLUSTER" \
  --type merge -p '{"spec":{"replicas":null,"autoscaling":{"minReplicas":2,"maxReplicas":5}}}'

echo "Autoscaling enabled on $POOL_NAME"

# ─────────────────────────────────────────────────────────────────────
# Step 5: Disable autoscaling (back to fixed)
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 5: Disable autoscaling ==="

kubectl patch machinepool.hive.openshift.io "$POOL_NAME" -n "$CLUSTER" \
  --type merge -p '{"spec":{"replicas":2,"autoscaling":null}}'

echo "Fixed at 2 replicas"


# ═════════════════════════════════════════════════════════════════════
# CREATE MACHINEPOOL (for clusters without one)
# ═════════════════════════════════════════════════════════════════════
# Some Hive-provisioned clusters may not have a MachinePool resource.
# Creating one lets Hive manage the workers.

# ─────────────────────────────────────────────────────────────────────
# Step 6: Detect current workers from ManagedClusterInfo
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 6: Detect current workers ==="

echo "Worker nodes:"
kubectl get managedclusterinfo -n "$CLUSTER" "$CLUSTER" \
  -o jsonpath='{range .status.nodeList[*]}{.name}: type={.labels.node\.kubernetes\.io/instance-type} role={.labels.node-role\.kubernetes\.io/worker}{"\n"}{end}'

# ─────────────────────────────────────────────────────────────────────
# Step 7: Create MachinePool matching existing workers
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 7: Create MachinePool ==="

cat <<EOF | kubectl apply -f -
apiVersion: hive.openshift.io/v1
kind: MachinePool
metadata:
  name: ${CLUSTER}-worker
  namespace: ${CLUSTER}
spec:
  clusterDeploymentRef:
    name: ${CLUSTER}
  name: worker
  replicas: 2
  platform:
    ${PLATFORM}:
      type: ${WORKER_TYPE}
EOF

echo "MachinePool ${CLUSTER}-worker created"


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab scaling list
# acmlab scaling get spoke2
# acmlab scaling set spoke2 --replicas 3
# acmlab scaling auto spoke2 --min 2 --max 5
# acmlab scaling init spoke2   # auto-detects workers and creates MachinePool
