#!/bin/bash
# UC-37: Worker Node Flavor Change via MachinePool
#
# Changes the instance type (flavor) of worker nodes on a Hive-provisioned
# cluster by patching the MachinePool. Hive performs a rolling replacement
# of the worker nodes.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Cluster provisioned via Hive (has MachinePool)
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER_NAME="spoke1"
NEW_FLAVOR="bx2-8x32"
POOL_NAME="${CLUSTER_NAME}-worker"


# ═════════════════════════════════════════════════════════════════════
# CHANGE FLAVOR
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Check current MachinePool
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Current MachinePool ==="

kubectl get machinepool "$POOL_NAME" -n "$CLUSTER_NAME" \
  -o jsonpath='Replicas: {.spec.replicas}, Type: {.spec.platform.ibmcloud.type}'
echo ""

# ─────────────────────────────────────────────────────────────────────
# Step 2: Patch instance type
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Change flavor to ${NEW_FLAVOR} ==="

kubectl patch machinepool "$POOL_NAME" -n "$CLUSTER_NAME" --type merge -p \
  '{"spec":{"platform":{"ibmcloud":{"type":"'"${NEW_FLAVOR}"'"}}}}'

# ─────────────────────────────────────────────────────────────────────
# Step 3: Monitor rolling replacement
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Monitor replacement ==="

kubectl get machinepool "$POOL_NAME" -n "$CLUSTER_NAME" \
  -o jsonpath='Type: {.spec.platform.ibmcloud.type}, Replicas: {.spec.replicas}'
echo ""

echo "Conditions:"
kubectl get machinepool "$POOL_NAME" -n "$CLUSTER_NAME" \
  -o jsonpath='{range .status.conditions[*]}{.type}: {.status} — {.message}{"\n"}{end}'


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab scaling set-flavor spoke1 --type bx2-8x32
# acmlab scaling get spoke1
