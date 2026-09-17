#!/usr/bin/env bash
# UC-45: Fleet right-sizing recommendations (MCOA)
# Demonstrates enabling right-sizing metrics, getting recommendations, and applying adjustments

set -euo pipefail

echo "=== UC-45: Right-Sizing ==="

echo "1. Enable right-sizing metrics collection on a cluster"
acmlab rightsizing enable spoke1

echo ""
echo "2. List right-sizing enabled clusters"
acmlab rightsizing list

echo ""
echo "3. Get right-sizing recommendations (advise mode — read-only)"
acmlab rightsizing advise spoke1

echo ""
echo "4. Apply recommendations with dry-run first"
acmlab rightsizing adjust spoke1 --dry-run

echo ""
echo "5. Apply recommendations for real"
acmlab rightsizing adjust spoke1

echo ""
echo "6. Disable right-sizing"
acmlab rightsizing disable spoke1

echo ""
echo "=== Done ==="
