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

echo "--- Step 3: Wait for compliance propagation (~${PROPAGATION_WAIT}s) ---"
sleep "$PROPAGATION_WAIT"

echo "--- Step 4: Check compliance status ---"
acmlab policy status "$POLICY_NAME"

echo "--- Step 5: Switch to enforce mode ---"
acmlab policy apply "$POLICY_NAME" \
  --registries "registry.redhat.io,quay.io,registry.access.redhat.com" \
  --remediation enforce

echo "--- Step 6: Wait and verify enforcement ---"
sleep "$PROPAGATION_WAIT"
acmlab policy status "$POLICY_NAME"

echo "--- Cleanup ---"
echo "To remove: acmlab policy remove $POLICY_NAME"
