#!/usr/bin/env bash
# UC-30: Policy automation — Ansible auto-remediation via PolicyAutomation
# Requires: Ansible Automation Platform credentials secret on the hub

set -euo pipefail

echo "=== UC-30: Policy Automation ==="

echo "1. Ensure a governance policy exists"
acmlab policy list

echo ""
echo "2. Create PolicyAutomation linking policy to Ansible job template"
acmlab automation create auto-remediate-images \
  --policy image-policy \
  --tower-secret tower-creds \
  --job-template remediate-registry-violations \
  --mode scan

echo ""
echo "3. Check automation status"
acmlab automation get auto-remediate-images

echo ""
echo "4. List all automations"
acmlab automation list

echo ""
echo "5. Switch to one-shot mode"
acmlab automation set-mode auto-remediate-images once

echo ""
echo "6. Disable automation"
acmlab automation set-mode auto-remediate-images disabled

echo ""
echo "7. Delete automation"
acmlab automation delete auto-remediate-images

echo ""
echo "=== Done ==="
