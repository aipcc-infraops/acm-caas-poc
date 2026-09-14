#!/usr/bin/env bash
# UC-04: Fleet status and cross-cluster search
# Demo: list fleet, inspect individual clusters, batch status
set -euo pipefail

echo "=== UC-04: Fleet Status ==="

echo "--- Step 1: List all managed clusters ---"
acmlab fleet list

echo "--- Step 2: Detailed status for a single cluster ---"
acmlab fleet status spoke2

echo "--- Step 3: Batch status (multiple clusters) ---"
acmlab fleet status spoke2 local-cluster

echo "--- Step 4: JSON output for automation ---"
acmlab fleet status spoke2 --json
