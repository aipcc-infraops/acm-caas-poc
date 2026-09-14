#!/usr/bin/env bash
# UC-10: Cluster scaling (add/remove worker nodes)
# Demo: get MachinePool, set replicas, enable autoscaling, init for new clusters
set -euo pipefail

CLUSTER="${1:-spoke2}"

echo "=== UC-10: Cluster Scaling ==="

echo "--- Step 1: List all MachinePools in fleet ---"
acmlab scaling list

echo "--- Step 2: Get MachinePool for $CLUSTER ---"
acmlab scaling get "$CLUSTER"

echo "--- Step 3: Scale to 3 workers ---"
acmlab scaling set "$CLUSTER" --replicas 3

echo "--- Step 4: Enable autoscaling (min=2, max=5) ---"
acmlab scaling auto "$CLUSTER" --min 2 --max 5

echo "--- Step 5: Disable autoscaling, fix at 2 ---"
acmlab scaling set "$CLUSTER" --replicas 2

echo "--- Step 6: JSON output ---"
acmlab scaling get "$CLUSTER" --json

echo ""
echo "--- Init example (for clusters without a MachinePool) ---"
echo "acmlab scaling init <cluster> auto-detects workers and creates a MachinePool:"
echo "  acmlab scaling init $CLUSTER"
echo "  acmlab scaling init $CLUSTER --worker-type bx2-8x32 --replicas 4"
