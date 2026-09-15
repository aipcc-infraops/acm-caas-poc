#!/usr/bin/env bash
# UC-24: Per-team compliance reporting via ClusterSet-scoped policies
# Demo: apply a policy scoped to a ClusterSet, generate compliance report
set -euo pipefail

TEAM="${1:-team-serving}"

echo "=== UC-24: Per-Team Compliance Reporting ==="

echo "--- Step 1: Apply policy scoped to $TEAM ClusterSet ---"
acmlab policy apply gpu-driver-check --cluster-set "$TEAM" --remediation inform

echo "--- Step 2: Check policy status ---"
acmlab policy status gpu-driver-check

echo "--- Step 3: Generate per-team compliance report ---"
acmlab policy report

echo "--- Step 4: JSON report ---"
acmlab policy report --json

echo "--- Step 5: Clean up ---"
echo "  acmlab policy remove gpu-driver-check"

echo "=== Demo complete ==="
