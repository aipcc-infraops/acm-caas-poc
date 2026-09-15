#!/usr/bin/env bash
# UC-28: Certificate expiry detection fleet-wide
# Demo: deploy CertificatePolicy, check compliance, clean up
set -euo pipefail

DAYS="${1:-30}"

echo "=== UC-28: Certificate Expiry Detection ==="

echo "--- Step 1: Apply CertificatePolicy (threshold: ${DAYS} days) ---"
acmlab policy apply cert-expiry-check \
  --cert-expiry "$DAYS" \
  --cert-namespaces "openshift-config,openshift-ingress"

echo "--- Step 2: Check compliance (shows clusters with expiring certs) ---"
acmlab policy status cert-expiry-check

echo "--- Step 3: Apply stricter policy (7 days) for critical namespaces ---"
acmlab policy apply cert-expiry-critical \
  --cert-expiry 7 \
  --cert-namespaces "openshift-config" \
  --remediation inform

echo "--- Step 4: List all policies ---"
acmlab policy list

echo "--- Step 5: Clean up ---"
echo "  acmlab policy remove cert-expiry-check"
echo "  acmlab policy remove cert-expiry-critical"

echo "=== Demo complete ==="
