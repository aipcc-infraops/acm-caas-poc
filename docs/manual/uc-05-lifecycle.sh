#!/bin/bash
# UC-05: Hibernate/Resume Lifecycle via Hive PowerState
#
# Hibernate and resume Hive-provisioned clusters by patching the
# ClusterDeployment spec.powerState field. Includes post-resume
# certificate recovery for expired kubelet CSRs.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Cluster provisioned by Hive (has a ClusterDeployment)
#   - For CSR approval: kubeconfig for the spoke cluster
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER="spoke2"
SPOKE_CONTEXT="spoke2"  # kubectl context for the spoke cluster


# ═════════════════════════════════════════════════════════════════════
# CHECK CURRENT STATE
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Read current power state
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Current power state ==="

echo "Desired (spec):"
kubectl get clusterdeployment -n "$CLUSTER" "$CLUSTER" \
  -o jsonpath='{.spec.powerState}'
echo ""

echo "Actual (status):"
kubectl get clusterdeployment -n "$CLUSTER" "$CLUSTER" \
  -o jsonpath='{.status.powerState}'
echo ""


# ═════════════════════════════════════════════════════════════════════
# HIBERNATE
# ═════════════════════════════════════════════════════════════════════
# Hibernation stops the cluster VMs. Takes ~10-15 minutes.
# The cluster becomes unavailable but is not destroyed.

# ─────────────────────────────────────────────────────────────────────
# Step 2: Patch powerState to Hibernating
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Hibernate ==="

kubectl patch clusterdeployment -n "$CLUSTER" "$CLUSTER" \
  --type merge -p '{"spec":{"powerState":"Hibernating"}}'

# ─────────────────────────────────────────────────────────────────────
# Step 3: Monitor transition
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Monitor hibernation ==="

echo "Waiting for status.powerState=Hibernating..."
while true; do
  state=$(kubectl get clusterdeployment -n "$CLUSTER" "$CLUSTER" \
    -o jsonpath='{.status.powerState}')
  echo "  status.powerState=$state"
  if [ "$state" = "Hibernating" ]; then
    echo "Cluster is hibernated."
    break
  fi
  sleep 30
done


# ═════════════════════════════════════════════════════════════════════
# RESUME
# ═════════════════════════════════════════════════════════════════════
# Resuming starts the cluster VMs. Takes ~10-20 minutes.

# ─────────────────────────────────────────────────────────────────────
# Step 4: Patch powerState to Running
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 4: Resume ==="

kubectl patch clusterdeployment -n "$CLUSTER" "$CLUSTER" \
  --type merge -p '{"spec":{"powerState":"Running"}}'

# ─────────────────────────────────────────────────────────────────────
# Step 5: Monitor transition
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 5: Monitor resume ==="

echo "Waiting for status.powerState=Running..."
while true; do
  state=$(kubectl get clusterdeployment -n "$CLUSTER" "$CLUSTER" \
    -o jsonpath='{.status.powerState}')
  echo "  status.powerState=$state"
  if [ "$state" = "Running" ]; then
    echo "Cluster is running."
    break
  fi
  sleep 30
done


# ═════════════════════════════════════════════════════════════════════
# POST-RESUME: CERTIFICATE RECOVERY
# ═════════════════════════════════════════════════════════════════════
# OpenShift kubelet client certs rotate every ~24h. If the cluster was
# hibernated during a rotation window, certs expire and nodes cannot
# start pods until the pending CSRs are approved.

# ─────────────────────────────────────────────────────────────────────
# Step 6: Check for pending CSRs on the spoke
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 6: Check for pending CSRs ==="

kubectl --context "$SPOKE_CONTEXT" get csr | grep Pending || echo "No pending CSRs"

# ─────────────────────────────────────────────────────────────────────
# Step 7: Approve all pending kubelet CSRs
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 7: Approve pending CSRs ==="

kubectl --context "$SPOKE_CONTEXT" get csr -o name | \
  xargs -I {} kubectl --context "$SPOKE_CONTEXT" certificate approve {}


# ═════════════════════════════════════════════════════════════════════
# DIAGNOSE
# ═════════════════════════════════════════════════════════════════════
# Cross-reference Hive and ACM state to detect inconsistencies.

# ─────────────────────────────────────────────────────────────────────
# Step 8: Compare Hive vs ACM state
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 8: Diagnose ==="

echo "Hive power state:"
kubectl get clusterdeployment -n "$CLUSTER" "$CLUSTER" \
  -o jsonpath='spec={.spec.powerState} status={.status.powerState}'
echo ""

echo "ACM availability:"
kubectl get managedcluster "$CLUSTER" \
  -o jsonpath='{range .status.conditions[?(@.type=="ManagedClusterConditionAvailable")]}{.type}: {.status}{end}'
echo ""


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab lifecycle status spoke2
# acmlab lifecycle hibernate spoke2 --wait
# acmlab lifecycle resume spoke2 --wait    # auto-approves expired CSRs
# acmlab lifecycle diagnose spoke2
# acmlab lifecycle list
