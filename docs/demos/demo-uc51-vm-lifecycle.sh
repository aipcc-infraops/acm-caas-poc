#!/usr/bin/env bash
# UC-51: VM lifecycle management (OpenShift Virtualization)
# Demonstrates deploy, start, stop, migrate, status, list, and remove VMs

set -euo pipefail

echo "=== UC-51: VM Lifecycle Management ==="

echo "1. Deploy a VM to a managed cluster"
acmlab vm deploy --name web-vm --cluster spoke1 --cpu 4 --memory 8Gi

echo ""
echo "2. Check VM status"
acmlab vm status web-vm --cluster spoke1

echo ""
echo "3. Stop the VM"
acmlab vm stop web-vm --cluster spoke1

echo ""
echo "4. Start the VM"
acmlab vm start web-vm --cluster spoke1

echo ""
echo "5. Trigger live migration"
acmlab vm migrate web-vm --cluster spoke1

echo ""
echo "6. List all VMs across clusters"
acmlab vm list

echo ""
echo "7. Remove the VM"
acmlab vm remove web-vm --cluster spoke1

echo ""
echo "=== Done ==="
