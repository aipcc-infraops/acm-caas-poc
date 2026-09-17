#!/usr/bin/env bash
# UC-50: ClusterCurator day-2 automation hooks
# Demonstrates creating curators with pre/post hooks for cluster lifecycle

set -euo pipefail

echo "=== UC-50: ClusterCurator Hooks ==="

echo "1. Create a ClusterCurator with pre and post hooks"
acmlab lifecycle curator apply spoke1 --pre-hook pre-upgrade-backup --post-hook post-upgrade-verify

echo ""
echo "2. Get curator status"
acmlab lifecycle curator status spoke1

echo ""
echo "3. List all curators"
acmlab lifecycle curator list

echo ""
echo "4. Remove curator"
acmlab lifecycle curator remove spoke1

echo ""
echo "=== Done ==="
