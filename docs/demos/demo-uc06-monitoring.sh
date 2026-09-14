#!/usr/bin/env bash
# UC-06: Cluster resource monitoring and observability (Thanos)
# Demo: list resources, inspect cluster, setup/teardown observability
set -euo pipefail

CLUSTER="${1:-spoke2}"

echo "=== UC-06: Monitoring & Observability ==="

echo "--- Step 1: List cluster resource summaries ---"
acmlab monitor list

echo "--- Step 2: Detailed resource info for $CLUSTER ---"
acmlab monitor status "$CLUSTER"

echo "--- Step 3: Deploy observability stack (MinIO + Thanos) ---"
echo "WARNING: This deploys real infrastructure on the hub."
echo "To deploy: acmlab monitor setup"

echo "--- Step 4: Check observability status ---"
echo "To check: acmlab monitor obs-status"

echo "--- Teardown ---"
echo "To remove: acmlab monitor teardown"
