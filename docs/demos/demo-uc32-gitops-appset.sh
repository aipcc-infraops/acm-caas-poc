#!/usr/bin/env bash
set -euo pipefail

echo "=== UC-32: GitOps Fleet Deployment via ApplicationSet ==="
echo ""

echo "--- Create ApplicationSet with placement generator ---"
echo "acmlab gitops create monitoring-stack \\"
echo "  --repo https://github.com/example/monitoring.git \\"
echo "  --path manifests/base \\"
echo "  --label env=prod"
echo ""

echo "--- Create ApplicationSet with cluster generator ---"
echo "acmlab gitops create logging-stack \\"
echo "  --repo https://github.com/example/logging.git \\"
echo "  --path k8s/overlays/prod \\"
echo "  --generator cluster \\"
echo "  --label tier=infra"
echo ""

echo "--- List all ApplicationSets ---"
echo "acmlab gitops list"
echo "acmlab gitops list --json"
echo ""

echo "--- Get ApplicationSet details ---"
echo "acmlab gitops get monitoring-stack"
echo "acmlab gitops get monitoring-stack --json"
echo ""

echo "--- Trigger sync ---"
echo "acmlab gitops sync monitoring-stack"
echo ""

echo "--- Delete ApplicationSet ---"
echo "acmlab gitops delete logging-stack"
echo ""

echo "=== MCP tools ==="
echo "acm_create_appset, acm_get_appset, acm_list_appsets, acm_delete_appset, acm_sync_appset"
