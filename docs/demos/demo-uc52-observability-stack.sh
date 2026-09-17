#!/usr/bin/env bash
# UC-52: Observability stack customisation
# Demonstrates pull secret, OBC storage, custom rules, dashboards, metrics, retention

set -euo pipefail

echo "=== UC-52: Observability Stack Customisation ==="

echo "1. Setup observability stack"
acmlab observability setup

echo ""
echo "2. Check status"
acmlab observability status

echo ""
echo "3. Configure pull secret"
acmlab observability configure-pull-secret

echo ""
echo "4. Configure OBC storage"
acmlab observability configure-storage --storage-class gp3-csi --bucket thanos

echo ""
echo "5. Deploy custom Prometheus rules"
acmlab observability deploy-rules --rules-file custom-rules.yaml

echo ""
echo "6. Deploy a custom Grafana dashboard"
acmlab observability deploy-dashboard --name gpu-overview --dashboard-file gpu-dashboard.json

echo ""
echo "7. Configure custom metrics allowlist"
acmlab observability configure-metrics --metric node_cpu_seconds_total --metric container_memory_rss

echo ""
echo "8. Check add-on health"
acmlab observability addon-health

echo ""
echo "9. Configure retention settings"
acmlab observability configure-retention --retention 24h --block-duration 2h --delete-delay 48h

echo ""
echo "10. Remove custom rules"
acmlab observability remove-rules

echo ""
echo "11. Remove custom dashboard"
acmlab observability remove-dashboard gpu-overview

echo ""
echo "12. Teardown observability stack"
acmlab observability teardown

echo ""
echo "=== Done ==="
