#!/usr/bin/env bash
# UC-05: Hibernate/resume lifecycle (Hive-only)
# Demo: hibernate, resume with cert recovery, diagnose
#
# This demo has phases because hibernate/resume take ~10-20 minutes each.
#
# Usage:
#   ./demo-uc05-lifecycle.sh status [cluster]       # Check current state
#   ./demo-uc05-lifecycle.sh hibernate [cluster]     # Start hibernation
#   ./demo-uc05-lifecycle.sh resume [cluster]        # Resume + cert recovery
#   ./demo-uc05-lifecycle.sh diagnose [cluster]      # Health diagnostics
set -euo pipefail

PHASE="${1:-status}"
CLUSTER="${2:-spoke2}"

echo "=== UC-05: Cluster Lifecycle ==="

case "$PHASE" in
  status)
    echo "--- Status ---"
    echo "Step 1: List clusters with lifecycle support"
    acmlab lifecycle list

    echo "Step 2: Current power state"
    acmlab lifecycle status "$CLUSTER"
    ;;

  hibernate)
    echo "--- Hibernate ---"
    echo "Step 1: Current power state"
    acmlab lifecycle status "$CLUSTER"

    echo "Step 2: Initiate hibernation"
    acmlab lifecycle hibernate "$CLUSTER"

    echo ""
    echo "Hibernation takes ~10-15 minutes."
    echo "Monitor with:  acmlab lifecycle status $CLUSTER"
    echo "When done:     $0 resume $CLUSTER"
    ;;

  resume)
    echo "--- Resume ---"
    echo "Step 1: Verify cluster is hibernated"
    acmlab lifecycle status "$CLUSTER"

    echo "Step 2: Resume the cluster (with --wait and cert recovery)"
    acmlab lifecycle resume "$CLUSTER" --wait --timeout 20m

    echo "Step 3: Post-resume diagnostics"
    acmlab lifecycle diagnose "$CLUSTER"
    ;;

  diagnose)
    echo "--- Diagnose ---"
    echo "Step 1: Text report"
    acmlab lifecycle diagnose "$CLUSTER"

    echo "Step 2: JSON report"
    acmlab lifecycle diagnose "$CLUSTER" --json
    ;;

  *)
    echo "Usage: $0 {status|hibernate|resume|diagnose} [cluster]"
    exit 1
    ;;
esac
