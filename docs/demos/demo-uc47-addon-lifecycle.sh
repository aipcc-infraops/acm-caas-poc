#!/usr/bin/env bash
# UC-47: Add-on lifecycle management (AddOnDeploymentConfig)
# Demonstrates listing, configuring, and managing add-ons

set -euo pipefail

echo "=== UC-47: Add-on Lifecycle Management ==="

echo "1. List all ClusterManagementAddOns"
acmlab addon list

echo ""
echo "2. Get details of a specific add-on"
acmlab addon get work-manager

echo ""
echo "3. Create an AddOnDeploymentConfig with custom values"
acmlab addon configure observability-config --set replica-count=3 --set log-level=debug

echo ""
echo "4. List AddOnDeploymentConfigs"
acmlab addon list-configs

echo ""
echo "5. Remove the config"
acmlab addon remove-config observability-config

echo ""
echo "=== Done ==="
