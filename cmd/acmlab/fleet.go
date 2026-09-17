package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/batch"
	"github.com/pablofelix/acm-caas-poc/internal/fleet"
)

func fleetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Query managed cluster fleet",
	}
	cmd.AddCommand(fleetListCmd(), fleetStatusCmd(), fleetScoringConfigureCmd(), fleetScoringStatusCmd(), fleetScoringRemoveCmd(), fleetScoringListCmd())
	return cmd
}

func fleetListCmd() *cobra.Command {
	var outputJSON bool
	var labelSelector string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all managed clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			insp := fleet.New(c, cfg, logger)
			clusters, err := insp.ListClusters(context.Background(), labelSelector)
			if err != nil {
				return err
			}
			if len(clusters) == 0 {
				fmt.Println("No managed clusters found")
				return nil
			}
			if outputJSON {
				data, _ := json.MarshalIndent(clusters, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("%-30s %-10s %-10s %-10s %s\n", "NAME", "AVAILABLE", "JOINED", "ACCEPTED", "VERSION")
			for _, cl := range clusters {
				fmt.Printf("%-30s %-10v %-10v %-10v %s\n",
					cl.Name, cl.Available, cl.Joined, cl.Accepted, cl.Version)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	cmd.Flags().StringVarP(&labelSelector, "label-selector", "l", "", "filter clusters by label (e.g. vendor=OpenShift)")
	return cmd
}

func fleetStatusCmd() *cobra.Command {
	var outputJSON bool
	var fromFile string
	var concurrency int

	cmd := &cobra.Command{
		Use:   "status [name...]",
		Short: "Show status of one or more clusters",
		Args:  cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			var fileItems []batch.ClusterItem
			if fromFile != "" {
				var err error
				fileItems, err = batch.LoadFile(fromFile)
				if err != nil {
					return err
				}
			}
			items, err := batch.NamesFromArgs(args, fileItems)
			if err != nil {
				return err
			}

			c, err := buildClient()
			if err != nil {
				return err
			}
			fi := fleet.New(c, cfg, logger)
			ctx := context.Background()

			// Single-cluster detailed output (original behaviour)
			if len(items) == 1 && fromFile == "" {
				info, err := fi.GetCluster(ctx, items[0].Name)
				if err != nil {
					return err
				}
				if outputJSON {
					data, _ := json.MarshalIndent(info, "", "  ")
					fmt.Println(string(data))
					return nil
				}
				fmt.Printf("Cluster: %s\n", info.Name)
				fmt.Printf("Version: %s\n", info.Version)
				fmt.Printf("Available: %v\n", info.Available)
				fmt.Printf("Joined: %v\n", info.Joined)
				fmt.Printf("Accepted: %v\n", info.Accepted)
				fmt.Printf("Labels:\n")
				for k, v := range info.Labels {
					fmt.Printf("  %s=%s\n", k, v)
				}
				fmt.Printf("Conditions:\n")
				for _, cond := range info.Conditions {
					fmt.Printf("  %-45s %-6s %s\n", cond.Type+":", cond.Status, cond.Message)
				}
				return nil
			}

			work := make([]batch.Work, len(items))
			for i, item := range items {
				item := item
				work[i] = batch.Work{
					Name: item.Name,
					Run: func(ctx context.Context) (string, error) {
						s, err := fi.GetCluster(ctx, item.Name)
						if err != nil {
							return "", err
						}
						return fmt.Sprintf("Available=%v Joined=%v Version=%s", s.Available, s.Joined, s.Version), nil
					},
				}
			}

			results := batch.Execute(ctx, work, concurrency, os.Stdout)
			if outputJSON {
				data, _ := batch.ToJSON(results)
				fmt.Println(string(data))
			} else {
				batch.PrintSummary(results, os.Stdout)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "YAML file with cluster list")
	cmd.Flags().IntVar(&concurrency, "concurrency", 5, "Max parallel operations (max 20)")
	return cmd
}

func fleetScoringConfigureCmd() *cobra.Command {
	var namespace, clusterSet, labels string
	var prioritizers []string

	cmd := &cobra.Command{
		Use:   "scoring-configure <name>",
		Short: "Configure placement scoring for cluster selection (UC-48)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			insp := fleet.New(c, cfg, logger)

			opts := fleet.ScoringOpts{
				Name:         args[0],
				Namespace:    namespace,
				Prioritizers: prioritizers,
				ClusterSet:   clusterSet,
			}
			if labels != "" {
				opts.Labels = parseLabelsFleet(labels)
			}

			fmt.Printf("Configuring placement scoring %s...\n", args[0])
			if err := insp.ConfigureScoring(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("Scoring placement configured. Clusters will be ranked by resource availability.")
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "placement namespace (default: open-cluster-management)")
	cmd.Flags().StringSliceVar(&prioritizers, "prioritizers", nil, "scoring prioritizers (default: ResourceAllocatableCPU,ResourceAllocatableMemory)")
	cmd.Flags().StringVar(&clusterSet, "cluster-set", "", "scope to a ClusterSet")
	cmd.Flags().StringVarP(&labels, "labels", "l", "", "filter clusters by labels (key=value,key2=value2)")
	return cmd
}

func fleetScoringStatusCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "scoring-status <name>",
		Short: "Show placement scoring decisions (UC-48)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			insp := fleet.New(c, cfg, logger)
			status, err := insp.GetScoring(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Scoring: %s\n", status.Name)
			fmt.Printf("Namespace: %s\n", status.Namespace)
			if len(status.Decisions) == 0 {
				fmt.Println("No placement decisions yet")
				return nil
			}
			fmt.Printf("\n%-30s %-10s %s\n", "CLUSTER", "SCORE", "REASON")
			for _, d := range status.Decisions {
				fmt.Printf("%-30s %-10d %s\n", d.Cluster, d.Score, d.Reason)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "placement namespace")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func fleetScoringRemoveCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "scoring-remove <name>",
		Short: "Remove a scoring placement (UC-48)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			insp := fleet.New(c, cfg, logger)
			fmt.Printf("Removing scoring placement %s...\n", args[0])
			if err := insp.RemoveScoring(context.Background(), args[0], namespace); err != nil {
				return err
			}
			fmt.Println("Scoring placement removed.")
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "placement namespace")
	return cmd
}

func fleetScoringListCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "scoring-list",
		Short: "List all scoring placements (UC-48)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			insp := fleet.New(c, cfg, logger)
			infos, err := insp.ListScoring(context.Background(), namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(infos, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(infos) == 0 {
				fmt.Println("No scoring placements found")
				return nil
			}
			fmt.Printf("%-30s %s\n", "NAME", "NAMESPACE")
			for _, info := range infos {
				fmt.Printf("%-30s %s\n", info.Name, info.Namespace)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "placement namespace")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func parseLabelsFleet(s string) map[string]string {
	labels := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			labels[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return labels
}
