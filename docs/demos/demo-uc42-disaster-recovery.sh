#!/usr/bin/env bash
# UC-42: Workload disaster recovery (Velero + GitOps)
# Demonstrates DR setup, failover, and status monitoring

set -euo pipefail

echo "=== UC-42: Disaster Recovery ==="

echo "1. Enable DR between cluster pair"
acmlab recovery enable-dr prod-east --target prod-west --schedule "0 */4 * * *"

echo ""
echo "2. List DR pairs"
acmlab recovery list-dr

echo ""
echo "3. Trigger failover to standby cluster"
acmlab recovery failover prod-east --target prod-west

echo ""
echo "4. Check failover status"
acmlab recovery failover-status prod-east --target prod-west

echo ""
echo "5. Check status as JSON"
acmlab recovery failover-status prod-east --target prod-west --json

echo ""
echo "6. Disable DR"
acmlab recovery disable-dr prod-east

echo ""
echo "=== Done ==="
