#!/usr/bin/env bash
# UC-16: Unique identity provider per cluster
# Demo: deploy unique credentials and enforce SSO compliance
set -euo pipefail

CLUSTER="${1:-spoke2}"

echo "=== UC-16: Unique IdP per Cluster ==="

echo "--- Step 1: Deploy unique emergency credentials ---"
acmlab idp configure-unique --cluster "$CLUSTER" --admin-user cluster-admin

echo "--- Step 2: Verify IdP deployed ---"
acmlab idp list --cluster "$CLUSTER"

echo "--- Step 3: Enforce SSO fleet-wide ---"
acmlab idp enforce-sso

echo "--- Step 4: Check compliance ---"
acmlab policy list

echo ""
echo "=== Demo complete ==="
echo "Cluster $CLUSTER has unique emergency credentials."
echo "SSO enforcement policy marks clusters without OpenID IdP as NonCompliant."
