#!/bin/bash
# UC-08: Legacy Cluster Decommissioning
#
# Safe multi-phase workflow to retire clusters from ACM management.
# State is tracked in a ConfigMap in the cluster's namespace (ADR-008).
#
# 7 phases: imported → audited → notified → backed-up → drained → deleted → cleaned
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Cluster must exist as a ManagedCluster in ACM
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER_NAME="legacy-cluster"
OWNER="team-alpha@example.com"
DEADLINE="2026-09-28T00:00:00Z"

# ═════════════════════════════════════════════════════════════════════
# STANDALONE AUDIT (read-only, no state change)
# ═════════════════════════════════════════════════════════════════════
# Collects node count, CPU/memory capacity, owner, platform from
# ManagedCluster labels and ManagedClusterInfo status.

echo "=== Standalone Audit ==="

# Using kubectl:
echo "--- ManagedCluster labels ---"
kubectl get managedcluster "$CLUSTER_NAME" -o jsonpath='{.metadata.labels}' | python3 -m json.tool

echo "--- ManagedClusterInfo node list ---"
kubectl get managedclusterinfo "$CLUSTER_NAME" -n "$CLUSTER_NAME" \
  -o jsonpath='{range .status.nodeList[*]}{.name}: cpu={.capacity.cpu}, memory={.capacity.memory}{"\n"}{end}'

# Using acmlab:
# acmlab decommission audit $CLUSTER_NAME


# ═════════════════════════════════════════════════════════════════════
# START DECOMMISSION (creates ConfigMap, runs audit → phase: audited)
# ═════════════════════════════════════════════════════════════════════

echo "=== Start Decommission ==="

# Using kubectl (manual ConfigMap creation):
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${CLUSTER_NAME}-decommission
  namespace: ${CLUSTER_NAME}
  labels:
    caas-poc/workflow: decommission
    caas-poc/cluster: ${CLUSTER_NAME}
data:
  phase: "imported"
  owner: "${OWNER}"
  deadline: "${DEADLINE}"
  history: '[{"phase":"imported","timestamp":"$(date -u +%Y-%m-%dT%H:%M:%SZ)","message":"Decommission workflow started"}]'
EOF

# Using acmlab (recommended — also runs audit automatically):
# acmlab decommission start $CLUSTER_NAME --owner "$OWNER" --deadline "$DEADLINE"


# ═════════════════════════════════════════════════════════════════════
# CHECK STATUS
# ═════════════════════════════════════════════════════════════════════

echo "=== Check Status ==="

# Using kubectl:
kubectl get configmap "${CLUSTER_NAME}-decommission" -n "$CLUSTER_NAME" -o jsonpath='{.data}' | python3 -m json.tool

# Using acmlab:
# acmlab decommission status $CLUSTER_NAME


# ═════════════════════════════════════════════════════════════════════
# LIST ALL ACTIVE DECOMMISSIONS
# ═════════════════════════════════════════════════════════════════════

echo "=== List Active Workflows ==="

# Using kubectl:
kubectl get configmap -A -l caas-poc/workflow=decommission \
  --no-headers -o custom-columns='CLUSTER:.metadata.labels.caas-poc/cluster,NAMESPACE:.metadata.namespace,PHASE:.data.phase'

# Using acmlab:
# acmlab decommission list


# ═════════════════════════════════════════════════════════════════════
# ADVANCE THROUGH PHASES
# ═════════════════════════════════════════════════════════════════════
# Each advance moves to the next phase. The acmlab CLI executes the
# phase action (notify, backup, drain, delete, cleanup) before moving.
#
# WARNING: advance past "drained" is DESTRUCTIVE — it deletes the cluster.

# Using acmlab (recommended — handles phase logic):
# acmlab decommission advance $CLUSTER_NAME   # audited → notified
# acmlab decommission advance $CLUSTER_NAME   # notified → backed-up
# acmlab decommission advance $CLUSTER_NAME   # backed-up → drained
# acmlab decommission advance $CLUSTER_NAME   # drained → deleted (DESTRUCTIVE)
# acmlab decommission advance $CLUSTER_NAME   # deleted → cleaned (removes ACM resources)

# Using kubectl (manual phase update — no action executed):
# kubectl patch configmap "${CLUSTER_NAME}-decommission" -n "$CLUSTER_NAME" \
#   --type merge -p '{"data":{"phase":"notified"}}'


# ═════════════════════════════════════════════════════════════════════
# CANCEL (keeps cluster intact, removes tracking ConfigMap)
# ═════════════════════════════════════════════════════════════════════

echo "=== Cancel (safe at any phase before deleted) ==="

# Using kubectl:
# kubectl delete configmap "${CLUSTER_NAME}-decommission" -n "$CLUSTER_NAME"

# Using acmlab:
# acmlab decommission cancel $CLUSTER_NAME


# ═════════════════════════════════════════════════════════════════════
# PHASE REFERENCE
# ═════════════════════════════════════════════════════════════════════
#
# Phase       Action                                              Reversible?
# ---------   ------------------------------------------------    -----------
# imported    Workflow created, ConfigMap stored                   Yes (cancel)
# audited     Node count, CPU, memory, owner collected            Yes (cancel)
# notified    Owner notification recorded (timestamp)             Yes (cancel)
# backed-up   Cluster state exported to YAML                      Yes (cancel)
# drained     Worker nodes cordoned + pods evicted                Partial
# deleted     ClusterDeployment/ManagedCluster deleted            NO
# cleaned     Namespace, ManifestWorks removed from hub           NO
