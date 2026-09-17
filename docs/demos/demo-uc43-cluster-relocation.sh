#!/usr/bin/env bash
# UC-43: Cluster relocation (planned workload migration)
# Demonstrates migration planning, execution, status, and rollback

set -euo pipefail

echo "=== UC-43: Cluster Relocation ==="

echo "1. Plan migration from source to target"
acmlab migration plan prod-east --target prod-west --namespaces app-ns,data-ns

echo ""
echo "2. List migration plans"
acmlab migration list

echo ""
echo "3. Execute the migration"
acmlab migration execute prod-east-to-prod-west

echo ""
echo "4. Check migration status"
acmlab migration status prod-east-to-prod-west

echo ""
echo "5. Status as JSON"
acmlab migration status prod-east-to-prod-west --json

echo ""
echo "6. Rollback the migration (if needed)"
acmlab migration rollback prod-east-to-prod-west

echo ""
echo "=== Done ==="
