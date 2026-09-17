package backup

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildBackupSchedule(opts BackupOpts) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "BackupSchedule",
			"metadata": map[string]interface{}{
				"name":      "acm-backup-schedule",
				"namespace": opts.Namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/backup":  "true",
				},
			},
			"spec": map[string]interface{}{
				"veleroSchedule":          opts.Schedule,
				"veleroTtl":               opts.VeleroTTL,
				"veleroStorageLocation":   opts.StorageLocation,
				"useManagedServiceAccount": true,
			},
		},
	}
}

func buildRestore(opts RestoreOpts) *unstructured.Unstructured {
	name := restoreName()
	if opts.BackupName != "" {
		name = "acm-restore-" + opts.BackupName
	}

	syncMode := opts.SyncMode
	if syncMode == "" {
		syncMode = "latest"
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "Restore",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": opts.Namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/backup":  "true",
				},
			},
			"spec": map[string]interface{}{
				"veleroManagedClustersBackupName": syncMode,
				"veleroCredentialsBackupName":     syncMode,
				"veleroResourcesBackupName":       syncMode,
			},
		},
	}
}
