#!/usr/bin/env bash
# UC-02: Governance policy management (image registry enforcement)
# Demo: create, wait for compliance propagation, enforce, remove
set -euo pipefail

POLICY_NAME="${1:-allowed-registries}"
PROPAGATION_WAIT="${PROPAGATION_WAIT:-15}"

echo "=== UC-02: Governance Policy ==="

echo "--- Step 1: List existing policies ---"
acmlab policy list

echo "--- Step 2: Apply an image registry restriction policy (inform mode) ---"
acmlab policy apply "$POLICY_NAME" \
  --registries "registry.redhat.io,quay.io,registry.access.redhat.com" \
  --remediation inform

echo "--- Step 3: Check compliance status (waits for propagation) ---"
acmlab policy status "$POLICY_NAME" --wait --timeout 2m

echo "--- Step 4: Switch to enforce mode ---"
acmlab policy apply "$POLICY_NAME" \
  --registries "registry.redhat.io,quay.io,registry.access.redhat.com" \
  --remediation enforce

echo "--- Step 5: Verify enforcement ---"
acmlab policy status "$POLICY_NAME" --wait --timeout 2m

echo "--- Cleanup ---"
echo "To remove: acmlab policy remove $POLICY_NAME"
