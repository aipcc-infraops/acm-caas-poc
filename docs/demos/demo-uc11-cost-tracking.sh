#!/usr/bin/env bash
# UC-11: Cost tracking — estimate cluster costs from node metadata
# Uses ManagedClusterInfo instance types + pricing table (ADR-009)

set -euo pipefail

echo "=== UC-11: Cost Tracking ==="

echo "1. Get cost estimate for a single cluster"
acmlab cost cluster spoke1

echo ""
echo "2. Get cost estimate as JSON"
acmlab cost cluster spoke1 --json

echo ""
echo "3. Stamp cost center on clusters"
acmlab cost stamp spoke1 engineering
acmlab cost stamp spoke2 data-science
acmlab cost stamp spoke3 engineering

echo ""
echo "4. Generate fleet-wide cost report"
acmlab cost report

echo ""
echo "5. Generate cost report as CSV"
acmlab cost report --format csv

echo ""
echo "6. Aggregate costs by cost center"
acmlab cost by-center

echo ""
echo "7. Aggregate costs as JSON"
acmlab cost by-center --json

echo ""
echo "8. Deploy cost dashboard to Grafana (requires ACM observability)"
echo "   - Configures metrics allowlist (kube_node_status_capacity, kube_node_info, kube_node_labels, kube_node_role)"
echo "   - Deploys recording rules using kube_node_role{role=\"worker\"} for accurate worker filtering"
echo "   - Creates Grafana dashboard with 7 panels: estimate, avg rate, daily trend, etc."
acmlab cost dashboard

echo ""
echo "9. Set budget for a cost center"
acmlab cost budget engineering 5000

echo ""
echo "10. Check all budgets for overages"
acmlab cost check-budgets

echo ""
echo "11. Remove cost dashboard (only removes cost rule group, preserves other custom rules)"
acmlab cost remove-dashboard

echo ""
echo "=== Done ==="
