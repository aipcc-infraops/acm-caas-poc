#!/usr/bin/env bash
# UC-44: Disconnected cluster GitOps (Argo CD Agent pull-based)
# Demonstrates agent-mode GitOps for edge/disconnected clusters

set -euo pipefail

echo "=== UC-44: Disconnected GitOps (Agent Mode) ==="

echo "1. Enable agent-mode GitOps for edge clusters"
acmlab gitops enable-agent edge-apps --repo https://github.com/example-org/edge-configs --path manifests/edge --cluster edge-01 --cluster edge-02

echo ""
echo "2. Check agent-mode status"
acmlab gitops agent-status edge-apps

echo ""
echo "3. List all GitOps deployments (includes agent-mode)"
acmlab gitops list

echo ""
echo "4. Disable agent-mode"
acmlab gitops disable-agent edge-apps

echo ""
echo "=== Done ==="
