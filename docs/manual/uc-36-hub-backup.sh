#!/usr/bin/env bash
# UC-36: Hub Backup and Restore — raw oc/kubectl commands
# Requires: OADP operator installed, Velero BSL configured

BACKUP_NS="open-cluster-management-backup"

# --- Enable hub backup ---
# Create a BackupSchedule CR to periodically back up ACM hub resources
cat <<'EOF' | oc apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: BackupSchedule
metadata:
  name: acm-backup-schedule
  namespace: open-cluster-management-backup
spec:
  veleroSchedule: "0 */6 * * *"
  veleroTtl: 720h
  veleroStorageLocation: default
  useManagedServiceAccount: true
EOF

# --- Check backup schedule status ---
oc get backupschedule -n "$BACKUP_NS" acm-backup-schedule -o yaml

# --- List Velero backups (created by the schedule) ---
oc get backups.velero.io -n "$BACKUP_NS" --sort-by=.metadata.creationTimestamp

# --- List ACM-specific backup resources ---
oc get backups.velero.io -n "$BACKUP_NS" -l cluster.open-cluster-management.io/backup-schedule-label=acm-backup-schedule

# --- Restore from latest backup ---
cat <<'EOF' | oc apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Restore
metadata:
  name: acm-restore-latest
  namespace: open-cluster-management-backup
spec:
  veleroManagedClustersBackupName: latest
  veleroCredentialsBackupName: latest
  veleroResourcesBackupName: latest
EOF

# --- Restore from specific backup ---
# First, find available backups:
oc get backups.velero.io -n "$BACKUP_NS" -o custom-columns=NAME:.metadata.name,STATUS:.status.phase,STARTED:.status.startTimestamp

# Then restore from a specific one:
cat <<EOF | oc apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Restore
metadata:
  name: acm-restore-specific
  namespace: open-cluster-management-backup
spec:
  veleroManagedClustersBackupName: acm-managed-clusters-schedule-20260916120000
  veleroCredentialsBackupName: acm-credentials-schedule-20260916120000
  veleroResourcesBackupName: acm-resources-schedule-20260916120000
EOF

# --- Check restore status ---
oc get restore -n "$BACKUP_NS" -o custom-columns=NAME:.metadata.name,PHASE:.status.phase

# --- Disable hub backup ---
oc delete backupschedule -n "$BACKUP_NS" acm-backup-schedule

# --- Verify OADP operator is installed ---
oc get csv -n openshift-adp -l operators.coreos.com/oadp-operator.openshift-adp

# --- Check Velero backup storage location ---
oc get backupstoragelocation -n "$BACKUP_NS"
