#!/usr/bin/env bash
# UC-49: PolicySet compliance profiles (golden config)
# Demonstrates grouping policies into compliance profiles with single placement

set -euo pipefail

echo "=== UC-49: PolicySet Compliance Profiles ==="

echo "1. Create a PolicySet grouping related policies"
acmlab policy create-policyset cis-golden-config --policies image-registry-policy,cert-expiry-policy,operator-version-policy --description "CIS Level 1 golden configuration"

echo ""
echo "2. Get PolicySet details"
acmlab policy get-policyset cis-golden-config

echo ""
echo "3. List all PolicySets"
acmlab policy list-policysets

echo ""
echo "4. Remove PolicySet"
acmlab policy remove-policyset cis-golden-config

echo ""
echo "=== Done ==="
