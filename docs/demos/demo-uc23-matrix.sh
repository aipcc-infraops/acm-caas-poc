#!/usr/bin/env bash
# UC-23: Multi-Architecture Cluster Matrix — Demo Script
# Provisions a matrix of clusters covering OCP versions x architectures x operator versions
set -euo pipefail

ACMLAB="${ACMLAB:-./bin/acmlab}"

echo "=== UC-23: Multi-Architecture Cluster Matrix ==="
echo ""

echo "--- Step 1: Provision a 2x2x1 matrix ---"
$ACMLAB matrix provision \
  --versions 4.18,4.19 \
  --archs amd64,arm64 \
  --operators 2.5
echo ""

echo "--- Step 2: List matrix clusters ---"
$ACMLAB matrix list
echo ""

echo "--- Step 3: Check matrix status (use the matrix ID from provision output) ---"
echo "(In a real demo, pass the matrix-id returned from provision)"
# $ACMLAB matrix status <matrix-id>
echo ""

echo "--- Step 4: Provision a larger matrix with 3 architectures ---"
$ACMLAB matrix provision \
  --versions 4.18,4.19 \
  --archs amd64,arm64,s390x \
  --operators 2.5,3.0
echo ""

echo "--- Step 5: List all matrix clusters ---"
$ACMLAB matrix list --json
echo ""

echo "--- Step 6: Destroy a matrix ---"
echo "(In a real demo, pass the matrix-id from provision)"
# $ACMLAB matrix destroy <matrix-id>
echo ""

echo "=== UC-23 demo complete ==="
