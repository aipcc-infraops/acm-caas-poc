package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/upgrade"
)

func upgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Manage OpenShift cluster version upgrades",
		Long: `View upgrade status, available updates, and trigger OCP version upgrades.

Supports Hive-provisioned and imported OpenShift clusters.
Vanilla Kubernetes clusters are report-only (version displayed but upgrades
must be performed through the provider's native tools).`,
	}
	cmd.AddCommand(
		upgradeStatusCmd(),
		upgradeListCmd(),
		upgradeSetChannelCmd(),
		upgradeStartCmd(),
		upgradeHistoryCmd(),
	)
	return cmd
}

func upgradeStatusCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "status <cluster>",
		Short: "Show upgrade status for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := upgrade.New(c, cfg, logger)
			ctx := context.Background()

			status, err := mgr.GetUpgradeStatus(ctx, args[0])
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Cluster:         %s\n", status.Cluster)
			fmt.Printf("Type:            %s\n", status.ClusterType)
			fmt.Printf("Upgrade method:  %s\n", status.UpgradeMethod)
			fmt.Printf("Current version: %s\n", status.CurrentVersion)
			if status.DesiredVersion != "" {
				fmt.Printf("Desired version: %s\n", status.DesiredVersion)
			}
			if status.Channel != "" {
				fmt.Printf("Channel:         %s\n", status.Channel)
			}
			if status.Progressing {
				fmt.Println("Status:          UPGRADING")
			}
			if status.UpgradeFailed {
				fmt.Println("Status:          FAILED")
				if status.FailureMessage != "" {
					fmt.Printf("Failure:         %s\n", status.FailureMessage)
				}
			}
			if len(status.Available) > 0 {
				fmt.Printf("Available:       %s\n", strings.Join(status.Available, ", "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func upgradeListCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List clusters with available upgrades",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := upgrade.New(c, cfg, logger)
			ctx := context.Background()

			clusters, err := mgr.ListUpgradeable(ctx)
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(clusters, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(clusters) == 0 {
				fmt.Println("No clusters with available upgrades")
				return nil
			}

			fmt.Printf("%-20s %-12s %-16s %-12s %s\n", "CLUSTER", "VERSION", "CHANNEL", "METHOD", "AVAILABLE")
			for _, cl := range clusters {
				avail := strings.Join(cl.Available, ", ")
				if len(avail) > 40 {
					avail = avail[:37] + "..."
				}
				fmt.Printf("%-20s %-12s %-16s %-12s %s\n",
					cl.Cluster, cl.CurrentVersion, cl.Channel, cl.UpgradeMethod, avail)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func upgradeSetChannelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-channel <cluster> <channel>",
		Short: "Set the OCP update channel for a cluster",
		Long:  "Sets the update channel (e.g., stable-4.16, fast-4.16, candidate-4.16) via ManifestWork.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := upgrade.New(c, cfg, logger)
			ctx := context.Background()

			if err := mgr.SetChannel(ctx, args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Channel set to %s for cluster %s\n", args[1], args[0])
			return nil
		},
	}
}

func upgradeStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <cluster> <version>",
		Short: "Start an upgrade to a specific version",
		Long:  "Triggers a cluster upgrade by creating a ManifestWork that patches the ClusterVersion desiredUpdate.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := upgrade.New(c, cfg, logger)
			ctx := context.Background()

			if err := mgr.StartUpgrade(ctx, args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Upgrade to %s started for cluster %s\n", args[1], args[0])
			return nil
		},
	}
}

func upgradeHistoryCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "history <cluster>",
		Short: "Show version upgrade history for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := upgrade.New(c, cfg, logger)
			ctx := context.Background()

			entries, err := mgr.GetHistory(ctx, args[0])
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(entries, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(entries) == 0 {
				fmt.Println("No upgrade history available")
				return nil
			}

			fmt.Printf("%-12s %-12s %-24s %-24s\n", "VERSION", "STATE", "STARTED", "COMPLETED")
			for _, e := range entries {
				completed := e.CompletedAt
				if completed == "" {
					completed = "-"
				}
				started := e.StartedAt
				if started == "" {
					started = "-"
				}
				fmt.Printf("%-12s %-12s %-24s %-24s\n", e.Version, e.State, started, completed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
