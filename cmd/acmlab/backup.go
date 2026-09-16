package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/backup"
)

func backupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Manage hub backup and restore via OADP/Velero",
	}
	cmd.AddCommand(backupEnableCmd(), backupDisableCmd(), backupStatusCmd(), backupListCmd(), backupRestoreCmd())
	return cmd
}

func backupEnableCmd() *cobra.Command {
	var schedule, ttl, storageLocation, namespace string

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable scheduled hub backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := backup.New(c, cfg, logger)
			opts := backup.BackupOpts{
				Namespace:       namespace,
				Schedule:        schedule,
				VeleroTTL:       ttl,
				StorageLocation: storageLocation,
			}
			fmt.Println("Enabling hub backup schedule...")
			if err := mgr.Enable(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("Hub backup schedule enabled.")
			return nil
		},
	}
	cmd.Flags().StringVar(&schedule, "schedule", "", "Cron schedule (default: every 6 hours)")
	cmd.Flags().StringVar(&ttl, "ttl", "", "Backup TTL (default: 720h)")
	cmd.Flags().StringVar(&storageLocation, "storage-location", "", "Velero BSL name (default: default)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Backup namespace")
	return cmd
}

func backupDisableCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable hub backup schedule",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := backup.New(c, cfg, logger)
			fmt.Println("Disabling hub backup schedule...")
			removed, err := mgr.Disable(context.Background(), namespace)
			if err != nil {
				return err
			}
			if removed {
				fmt.Println("Hub backup schedule disabled.")
			} else {
				fmt.Println("No backup schedule found (nothing to disable)")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Backup namespace")
	return cmd
}

func backupStatusCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show hub backup schedule status",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := backup.New(c, cfg, logger)
			status, err := mgr.GetStatus(context.Background(), namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Enabled:          %v\n", status.Enabled)
			if status.Enabled {
				fmt.Printf("Schedule:         %s\n", status.Schedule)
				fmt.Printf("Phase:            %s\n", status.Phase)
				fmt.Printf("Storage Location: %s\n", status.StorageLocation)
				if status.LastBackup != "" {
					fmt.Printf("Last Backup:      %s\n", status.LastBackup)
					fmt.Printf("Last Status:      %s\n", status.LastStatus)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Backup namespace")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func backupListCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List completed backup/restore operations",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := backup.New(c, cfg, logger)
			infos, err := mgr.ListBackups(context.Background(), namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(infos, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(infos) == 0 {
				fmt.Println("No backup/restore operations found")
				return nil
			}
			fmt.Printf("%-35s %-15s %s\n", "NAME", "PHASE", "START TIME")
			for _, b := range infos {
				fmt.Printf("%-35s %-15s %s\n", b.Name, b.Phase, b.StartTime)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Backup namespace")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func backupRestoreCmd() *cobra.Command {
	var namespace, backupName, syncMode string

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Trigger hub restore from a backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := backup.New(c, cfg, logger)
			opts := backup.RestoreOpts{
				Namespace:  namespace,
				BackupName: backupName,
				SyncMode:   syncMode,
			}
			fmt.Println("Initiating hub restore...")
			if err := mgr.Restore(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("Hub restore initiated. Monitor with 'acmlab backup list'.")
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Backup namespace")
	cmd.Flags().StringVar(&backupName, "backup-name", "", "Specific backup to restore from")
	cmd.Flags().StringVar(&syncMode, "sync-mode", "", "Sync mode: latest, skip (default: latest)")
	return cmd
}
