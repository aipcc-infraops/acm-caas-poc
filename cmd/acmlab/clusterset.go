package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/clusterset"
)

func clustersetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clusterset",
		Short: "Manage ManagedClusterSets for team-based fleet isolation",
	}
	cmd.AddCommand(
		clustersetCreateCmd(),
		clustersetRemoveCmd(),
		clustersetListCmd(),
		clustersetAssignCmd(),
		clustersetGlobalEnableCmd(),
		clustersetGlobalBindCmd(),
		clustersetGlobalUnbindCmd(),
		clustersetGlobalStatusCmd(),
	)
	return cmd
}

func clustersetCreateCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a ClusterSet with binding in a team namespace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" {
				return fmt.Errorf("--namespace is required (team namespace for the binding)")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			if err := mgr.Create(context.Background(), args[0], namespace); err != nil {
				return err
			}
			fmt.Printf("ClusterSet %s created with binding in namespace %s\n", args[0], namespace)
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "team namespace for the ClusterSetBinding (required)")
	return cmd
}

func clustersetRemoveCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a ClusterSet and its binding",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" {
				return fmt.Errorf("--namespace is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			if err := mgr.Remove(context.Background(), args[0], namespace); err != nil {
				return err
			}
			fmt.Printf("ClusterSet %s removed\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "namespace of the ClusterSetBinding (required)")
	return cmd
}

func clustersetListCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all ClusterSets with member counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			sets, err := mgr.List(context.Background())
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(sets, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(sets) == 0 {
				fmt.Println("No ClusterSets found")
				return nil
			}
			fmt.Printf("%-30s %-8s %s\n", "NAME", "COUNT", "MEMBERS")
			for _, s := range sets {
				members := ""
				for i, m := range s.Members {
					if i > 0 {
						members += ", "
					}
					members += m
				}
				fmt.Printf("%-30s %-8d %s\n", s.Name, s.Count, members)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func clustersetAssignCmd() *cobra.Command {
	var setName string
	cmd := &cobra.Command{
		Use:   "assign <cluster>",
		Short: "Assign a cluster to a ClusterSet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if setName == "" {
				return fmt.Errorf("--to is required (target ClusterSet name)")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			if err := mgr.Assign(context.Background(), args[0], setName); err != nil {
				return err
			}
			fmt.Printf("Cluster %s assigned to ClusterSet %s\n", args[0], setName)
			return nil
		},
	}
	cmd.Flags().StringVar(&setName, "to", "", "target ClusterSet name (required)")
	return cmd
}

func clustersetGlobalEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "global-enable",
		Short: "Create or ensure the global ManagedClusterSet (matches all clusters)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			if err := mgr.EnableGlobal(context.Background()); err != nil {
				return err
			}
			fmt.Println("Global ManagedClusterSet enabled.")
			return nil
		},
	}
}

func clustersetGlobalBindCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "global-bind <namespace>",
		Short: "Bind the global ClusterSet to a namespace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			if err := mgr.BindGlobal(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Global ClusterSet bound to namespace %s\n", args[0])
			return nil
		},
	}
	return cmd
}

func clustersetGlobalUnbindCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "global-unbind <namespace>",
		Short: "Remove the global ClusterSet binding from a namespace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			if err := mgr.UnbindGlobal(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Global ClusterSet unbound from namespace %s\n", args[0])
			return nil
		},
	}
}

func clustersetGlobalStatusCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "global-status",
		Short: "Show status and bindings of the global ManagedClusterSet",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := clusterset.New(c, cfg, logger)
			status, err := mgr.GlobalStatus(context.Background())
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Enabled: %v\n", status.Enabled)
			if len(status.Bindings) > 0 {
				fmt.Println("Bindings:")
				for _, b := range status.Bindings {
					fmt.Printf("  - %s\n", b.Namespace)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
