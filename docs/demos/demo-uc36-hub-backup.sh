#!/usr/bin/env bash
set -euo pipefail

echo "=== UC-36: Hub Backup and Restore ==="
echo ""

echo "--- Enable hub backup (every 6 hours, 30-day retention) ---"
echo '$ acmlab backup enable --schedule "0 */6 * * *" --ttl 720h'
acmlab backup enable --schedule "0 */6 * * *" --ttl 720h
echo ""

echo "--- Check backup status ---"
echo '$ acmlab backup status'
acmlab backup status
echo ""

echo "--- Check backup status (JSON) ---"
echo '$ acmlab backup status --json'
acmlab backup status --json
echo ""

echo "--- List backup/restore operations ---"
echo '$ acmlab backup list'
acmlab backup list
echo ""

echo "--- Restore from latest backup ---"
echo '$ acmlab backup restore --sync-mode latest'
acmlab backup restore --sync-mode latest
echo ""

echo "--- Restore from specific backup ---"
echo '$ acmlab backup restore --backup-name acm-managed-clusters-schedule-20260916120000'
acmlab backup restore --backup-name acm-managed-clusters-schedule-20260916120000
echo ""

echo "--- Disable hub backup ---"
echo '$ acmlab backup disable'
acmlab backup disable
echo ""

echo "=== UC-36 Demo Complete ==="
