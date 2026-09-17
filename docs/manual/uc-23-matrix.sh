#!/usr/bin/env bash
# UC-23: Multi-Architecture Cluster Matrix — Manual Verification Script
set -euo pipefail

ACMLAB="${ACMLAB:-./bin/acmlab}"

echo "=== UC-23: Multi-Architecture Cluster Matrix — Manual Verification ==="
echo ""

echo "1. Provision a simple 2x2x1 matrix:"
echo "   $ACMLAB matrix provision --versions 4.18,4.19 --archs amd64,arm64 --operators 2.5"
echo ""

echo "2. Verify ClusterDeployments were created (4 expected):"
echo "   oc get clusterdeployments -A -l acmlab.redhat.com/matrix=true"
echo ""

echo "3. Verify labels on each ClusterDeployment:"
echo "   oc get clusterdeployments -A -l ocp-version=4.18,arch=amd64"
echo "   oc get clusterdeployments -A -l ocp-version=4.19,arch=arm64"
echo ""

echo "4. List matrix clusters via CLI:"
echo "   $ACMLAB matrix list"
echo ""

echo "5. Check matrix status:"
echo "   $ACMLAB matrix status <matrix-id>"
echo ""

echo "6. Provision a larger matrix (3 archs x 2 versions x 2 operators = 12 cells):"
echo "   $ACMLAB matrix provision --versions 4.18,4.19 --archs amd64,arm64,s390x --operators 2.5,3.0"
echo ""

echo "7. Verify 12 ClusterDeployments:"
echo "   oc get clusterdeployments -A -l acmlab.redhat.com/matrix=true | wc -l"
echo ""

echo "8. Destroy a matrix:"
echo "   $ACMLAB matrix destroy <matrix-id>"
echo ""

echo "9. Verify cleanup:"
echo "   oc get clusterdeployments -A -l matrix-id=<matrix-id>"
echo ""

echo "=== Verification complete ==="
