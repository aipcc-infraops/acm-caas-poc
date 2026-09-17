package backup

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const DefaultNamespace = "open-cluster-management-backup"

type BackupOpts struct {
	Namespace       string
	Schedule        string
	VeleroTTL       string
	StorageLocation string
}

type RestoreOpts struct {
	Namespace  string
	BackupName string
	SyncMode   string
}

type BackupStatus struct {
	Enabled         bool   `json:"enabled"`
	Schedule        string `json:"schedule,omitempty"`
	LastBackup      string `json:"lastBackup,omitempty"`
	LastStatus      string `json:"lastStatus,omitempty"`
	Phase           string `json:"phase,omitempty"`
	StorageLocation string `json:"storageLocation,omitempty"`
}

type BackupInfo struct {
	Name      string `json:"name"`
	Phase     string `json:"phase"`
	StartTime string `json:"startTime,omitempty"`
	TTL       string `json:"ttl,omitempty"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) applyDefaults(opts *BackupOpts) {
	if opts.Namespace == "" {
		opts.Namespace = DefaultNamespace
	}
	if opts.Schedule == "" {
		opts.Schedule = "0 */6 * * *"
	}
	if opts.VeleroTTL == "" {
		opts.VeleroTTL = "720h"
	}
	if opts.StorageLocation == "" {
		opts.StorageLocation = "default"
	}
}

func (m *Manager) Enable(ctx context.Context, opts BackupOpts) error {
	m.logger.Info("backup.Enable", "namespace", opts.Namespace)
	m.applyDefaults(&opts)

	schedule := buildBackupSchedule(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRBackupSchedule, opts.Namespace, schedule); err != nil {
		return fmt.Errorf("creating BackupSchedule: %w", err)
	}
	return nil
}

func (m *Manager) Disable(ctx context.Context, namespace string) (bool, error) {
	m.logger.Info("backup.Disable", "namespace", namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	_, err := m.client.Get(ctx, client.GVRBackupSchedule, namespace, "acm-backup-schedule")
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking BackupSchedule: %w", err)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRBackupSchedule, namespace, "acm-backup-schedule"); err != nil {
		return false, fmt.Errorf("deleting BackupSchedule: %w", err)
	}
	return true, nil
}

func (m *Manager) GetStatus(ctx context.Context, namespace string) (*BackupStatus, error) {
	m.logger.Info("backup.GetStatus", "namespace", namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	obj, err := m.client.Get(ctx, client.GVRBackupSchedule, namespace, "acm-backup-schedule")
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &BackupStatus{Enabled: false}, nil
		}
		return nil, fmt.Errorf("getting BackupSchedule: %w", err)
	}

	return parseBackupStatus(obj.Object), nil
}

func (m *Manager) ListBackups(ctx context.Context, namespace string) ([]BackupInfo, error) {
	m.logger.Info("backup.ListBackups", "namespace", namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	list, err := m.client.List(ctx, client.GVRRestore, namespace, "acmlab.redhat.com/backup")
	if err != nil {
		return nil, fmt.Errorf("listing backups: %w", err)
	}

	infos := make([]BackupInfo, 0, len(list.Items))
	for _, item := range list.Items {
		infos = append(infos, parseBackupInfo(item.Object))
	}
	return infos, nil
}

func (m *Manager) Restore(ctx context.Context, opts RestoreOpts) error {
	m.logger.Info("backup.Restore", "namespace", opts.Namespace, "backupName", opts.BackupName)
	if opts.Namespace == "" {
		opts.Namespace = DefaultNamespace
	}
	if opts.SyncMode == "" {
		opts.SyncMode = "latest"
	}

	restore := buildRestore(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRRestore, opts.Namespace, restore); err != nil {
		return fmt.Errorf("creating Restore: %w", err)
	}
	return nil
}

func parseBackupStatus(obj map[string]interface{}) *BackupStatus {
	bs := &BackupStatus{Enabled: true}

	spec, _ := obj["spec"].(map[string]interface{})
	if spec != nil {
		bs.Schedule, _ = spec["veleroSchedule"].(string)
		bs.StorageLocation, _ = spec["veleroStorageLocation"].(string)
	}

	status, _ := obj["status"].(map[string]interface{})
	if status != nil {
		bs.Phase, _ = status["phase"].(string)
		bs.LastBackup, _ = status["lastBackupTimestamp"].(string)
		bs.LastStatus, _ = status["lastBackupStatus"].(string)
	}

	return bs
}

func parseBackupInfo(obj map[string]interface{}) BackupInfo {
	info := BackupInfo{}
	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		info.Name, _ = meta["name"].(string)
	}
	if status, ok := obj["status"].(map[string]interface{}); ok {
		info.Phase, _ = status["phase"].(string)
		info.StartTime, _ = status["startTimestamp"].(string)
	}
	if spec, ok := obj["spec"].(map[string]interface{}); ok {
		info.TTL, _ = spec["veleroTtl"].(string)
	}
	return info
}

func restoreName() string {
	return fmt.Sprintf("acm-restore-%d", time.Now().Unix())
}
