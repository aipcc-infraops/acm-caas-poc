package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/submariner"
)

func submarinerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submariner",
		Short: "Multi-cluster networking via Submariner (UC-26)",
	}
	cmd.AddCommand(
		submarinerEnableCmd(),
		submarinerDisableCmd(),
		submarinerStatusCmd(),
		submarinerListCmd(),
	)
	return cmd
}

func submarinerEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <cluster-set>",
		Short: "Enable Submariner connectivity for a ClusterSet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := submariner.New(c, cfg, logger)
			fmt.Printf("Enabling Submariner for ClusterSet %s...\n", args[0])
			if err := mgr.Enable(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println("Submariner enabled. ManagedClusterAddOn + SubmarinerConfig created for all clusters in set.")
			return nil
		},
	}
}

func submarinerDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <cluster-set>",
		Short: "Disable Submariner for a ClusterSet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := submariner.New(c, cfg, logger)
			fmt.Printf("Disabling Submariner for ClusterSet %s...\n", args[0])
			if err := mgr.Disable(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println("Submariner disabled. AddOns and configs removed.")
			return nil
		},
	}
}

func submarinerStatusCmd() *cobra.Command {
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "status <cluster-set>",
		Short: "Show Submariner connectivity status for a ClusterSet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := submariner.New(c, cfg, logger)
			status, err := mgr.Status(context.Background(), args[0])
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(status)
			}
			conn := "CONNECTED"
			if !status.Connected {
				conn = "DISCONNECTED"
			}
			fmt.Printf("ClusterSet: %s  Status: %s\n", status.ClusterSet, conn)
			for _, cs := range status.Clusters {
				gw := "not-ready"
				if cs.GatewayReady {
					gw = "ready"
				}
				agent := "not-ready"
				if cs.AgentReady {
					agent = "ready"
				}
				fmt.Printf("  %-20s gateway=%-10s agent=%-10s connections=%d\n", cs.Name, gw, agent, cs.Connections)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "JSON output")
	return cmd
}

func submarinerListCmd() *cobra.Command {
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all Submariner-enabled cluster sets",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := submariner.New(c, cfg, logger)
			infos, err := mgr.List(context.Background())
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(infos)
			}
			if len(infos) == 0 {
				fmt.Println("No Submariner-enabled clusters found.")
				return nil
			}
			fmt.Printf("%-20s %-10s %-10s\n", "CLUSTER-SET", "CLUSTERS", "ENABLED")
			for _, info := range infos {
				fmt.Printf("%-20s %-10d %-10t\n", info.ClusterSet, info.Clusters, info.Enabled)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "JSON output")
	return cmd
}
