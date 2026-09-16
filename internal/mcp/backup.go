package mcp

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/pablofelix/acm-caas-poc/internal/backup"
)

func registerBackupTools(s *server.MCPServer, mgr *backup.Manager) {
	s.AddTool(
		mcp.NewTool("acm_enable_backup",
			mcp.WithDescription("Enable scheduled hub backup via OADP/Velero. Creates a BackupSchedule CR to periodically back up ACM hub resources (ManagedClusters, Policies, ManifestWorks, Placements). Idempotent."),
			mcp.WithString("schedule", mcp.Description("Cron schedule (default: every 6 hours)")),
			mcp.WithString("ttl", mcp.Description("Backup TTL (default: 720h)")),
			mcp.WithString("storage_location", mcp.Description("Velero BSL name (default: default)")),
			mcp.WithString("namespace", mcp.Description("Backup namespace (default: open-cluster-management-backup)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			opts := backup.BackupOpts{}
			opts.Schedule, _ = req.GetArguments()["schedule"].(string)
			opts.VeleroTTL, _ = req.GetArguments()["ttl"].(string)
			opts.StorageLocation, _ = req.GetArguments()["storage_location"].(string)
			opts.Namespace, _ = req.GetArguments()["namespace"].(string)
			if err := mgr.Enable(ctx, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("Hub backup schedule enabled"), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_disable_backup",
			mcp.WithDescription("Disable hub backup schedule. Deletes the BackupSchedule CR. Idempotent."),
			mcp.WithString("namespace", mcp.Description("Backup namespace (default: open-cluster-management-backup)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ns, _ := req.GetArguments()["namespace"].(string)
			removed, err := mgr.Disable(ctx, ns)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if !removed {
				return mcp.NewToolResultText("No backup schedule found (nothing to disable)"), nil
			}
			return mcp.NewToolResultText("Hub backup schedule disabled"), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_backup_status",
			mcp.WithDescription("Get hub backup schedule status — enabled state, schedule, last backup time, and phase."),
			mcp.WithString("namespace", mcp.Description("Backup namespace (default: open-cluster-management-backup)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ns, _ := req.GetArguments()["namespace"].(string)
			status, err := mgr.GetStatus(ctx, ns)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(status, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_list_backups",
			mcp.WithDescription("List completed backup/restore operations with phase and timestamps."),
			mcp.WithString("namespace", mcp.Description("Backup namespace (default: open-cluster-management-backup)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ns, _ := req.GetArguments()["namespace"].(string)
			infos, err := mgr.ListBackups(ctx, ns)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			data, _ := json.MarshalIndent(infos, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("acm_restore_backup",
			mcp.WithDescription("Trigger hub restore from a backup. Creates a Restore CR that restores ManagedClusters, credentials, and resources from the specified or latest backup."),
			mcp.WithString("backup_name", mcp.Description("Specific backup to restore from")),
			mcp.WithString("sync_mode", mcp.Description("Sync mode: latest, skip (default: latest)")),
			mcp.WithString("namespace", mcp.Description("Backup namespace (default: open-cluster-management-backup)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			opts := backup.RestoreOpts{}
			opts.BackupName, _ = req.GetArguments()["backup_name"].(string)
			opts.SyncMode, _ = req.GetArguments()["sync_mode"].(string)
			opts.Namespace, _ = req.GetArguments()["namespace"].(string)
			if err := mgr.Restore(ctx, opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("Hub restore initiated"), nil
		},
	)
}
