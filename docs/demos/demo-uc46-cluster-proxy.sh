#!/usr/bin/env bash
# UC-46: Cluster Proxy (spoke service exposure to hub)
# Demonstrates enabling reverse proxy tunnels for spoke clusters

set -euo pipefail

echo "=== UC-46: Cluster Proxy ==="

echo "1. Enable cluster proxy on spoke"
acmlab access enable-proxy spoke1

echo ""
echo "2. Check proxy status"
acmlab access proxy-status spoke1

echo ""
echo "3. Status as JSON"
acmlab access proxy-status spoke1 --json

echo ""
echo "4. Disable cluster proxy"
acmlab access disable-proxy spoke1

echo ""
echo "=== Done ==="
