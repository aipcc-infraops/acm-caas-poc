#!/usr/bin/env bash
# UC-48: Placement scoring (resource-based workload scheduling)
# Demonstrates configuring Placement prioritizers for CPU/Memory scoring

set -euo pipefail

echo "=== UC-48: Placement Scoring ==="

echo "1. Configure placement scoring with CPU and memory prioritizers"
acmlab fleet scoring configure gpu-placement --prioritizer ResourceAllocatableCPU --prioritizer ResourceAllocatableMemory

echo ""
echo "2. Get scoring status"
acmlab fleet scoring status gpu-placement

echo ""
echo "3. List all scoring configurations"
acmlab fleet scoring list

echo ""
echo "4. Remove scoring configuration"
acmlab fleet scoring remove gpu-placement

echo ""
echo "=== Done ==="
