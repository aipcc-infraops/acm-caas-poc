package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/observability"
)

func observabilityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "observability",
		Short: "Manage multicluster observability stack (MCO, Thanos, Grafana)",
	}
	cmd.AddCommand(
		observabilitySetupCmd(),
		observabilityTeardownCmd(),
		observabilityStatusCmd(),
		observabilityPullSecretCmd(),
		observabilityStorageCmd(),
		observabilityDeployRulesCmd(),
		observabilityRemoveRulesCmd(),
		observabilityDeployDashboardCmd(),
		observabilityRemoveDashboardCmd(),
		observabilityMetricsCmd(),
		observabilityAddonHealthCmd(),
		observabilityRetentionCmd(),
	)
	return cmd
}

func observabilitySetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Deploy the observability stack (MinIO, Thanos, MCO)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			fmt.Println("Setting up observability stack...")
			if err := mgr.Setup(context.Background()); err != nil {
				return err
			}
			fmt.Println("Observability stack deployed.")
			return nil
		},
	}
}

func observabilityTeardownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "teardown",
		Short: "Remove the observability stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			fmt.Println("Tearing down observability stack...")
			if err := mgr.Teardown(context.Background()); err != nil {
				return err
			}
			fmt.Println("Observability stack removed.")
			return nil
		},
	}
}

func observabilityStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show observability stack status",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			status, err := mgr.Status(context.Background())
			if err != nil {
				return err
			}
			fmt.Printf("Observability: %s\n", status)
			return nil
		},
	}
}

func observabilityPullSecretCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "configure-pull-secret",
		Short: "Copy pull secret from openshift-config to observability namespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			fmt.Println("Configuring pull secret...")
			if err := mgr.ConfigurePullSecret(context.Background()); err != nil {
				return err
			}
			fmt.Println("Pull secret configured.")
			return nil
		},
	}
}

func observabilityStorageCmd() *cobra.Command {
	var storageClass, bucket string

	cmd := &cobra.Command{
		Use:   "configure-storage",
		Short: "Configure ObjectBucketClaim for Thanos storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			opts := observability.StorageOpts{
				Type:         "obc",
				BucketName:   bucket,
				StorageClass: storageClass,
			}
			fmt.Println("Configuring OBC storage...")
			if err := mgr.ConfigureOBCStorage(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("OBC storage configured.")
			return nil
		},
	}
	cmd.Flags().StringVar(&storageClass, "storage-class", "", "storage class for the OBC")
	cmd.Flags().StringVar(&bucket, "bucket", "", "bucket name prefix")
	return cmd
}

func observabilityDeployRulesCmd() *cobra.Command {
	var rulesFile string

	cmd := &cobra.Command{
		Use:   "deploy-rules",
		Short: "Deploy custom Prometheus recording/alerting rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			if rulesFile == "" {
				return fmt.Errorf("--rules-file is required")
			}
			data, err := os.ReadFile(rulesFile)
			if err != nil {
				return fmt.Errorf("reading rules file: %w", err)
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			fmt.Println("Deploying custom rules...")
			if err := mgr.DeployCustomRules(context.Background(), observability.CustomRuleOpts{Rules: string(data)}); err != nil {
				return err
			}
			fmt.Println("Custom rules deployed.")
			return nil
		},
	}
	cmd.Flags().StringVar(&rulesFile, "rules-file", "", "path to YAML file with Prometheus rules")
	return cmd
}

func observabilityRemoveRulesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-rules",
		Short: "Remove custom Prometheus rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			if err := mgr.RemoveCustomRules(context.Background()); err != nil {
				return err
			}
			fmt.Println("Custom rules removed.")
			return nil
		},
	}
}

func observabilityDeployDashboardCmd() *cobra.Command {
	var name, dashFile string

	cmd := &cobra.Command{
		Use:   "deploy-dashboard",
		Short: "Deploy a custom Grafana dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" || dashFile == "" {
				return fmt.Errorf("--name and --dashboard-file are required")
			}
			data, err := os.ReadFile(dashFile)
			if err != nil {
				return fmt.Errorf("reading dashboard file: %w", err)
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			fmt.Printf("Deploying dashboard %s...\n", name)
			if err := mgr.DeployDashboard(context.Background(), observability.DashboardOpts{Name: name, JSON: string(data)}); err != nil {
				return err
			}
			fmt.Println("Dashboard deployed.")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "dashboard name (required)")
	cmd.Flags().StringVar(&dashFile, "dashboard-file", "", "path to Grafana dashboard JSON file")
	return cmd
}

func observabilityRemoveDashboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-dashboard <name>",
		Short: "Remove a custom Grafana dashboard",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			if err := mgr.RemoveDashboard(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Dashboard %s removed.\n", args[0])
			return nil
		},
	}
	return cmd
}

func observabilityMetricsCmd() *cobra.Command {
	var metrics []string

	cmd := &cobra.Command{
		Use:   "configure-metrics",
		Short: "Configure custom metrics allowlist for collection",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(metrics) == 0 {
				return fmt.Errorf("at least one --metric is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			fmt.Println("Configuring metrics allowlist...")
			if err := mgr.ConfigureMetricsAllowlist(context.Background(), observability.MetricsOpts{Metrics: metrics}); err != nil {
				return err
			}
			fmt.Println("Metrics allowlist configured.")
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&metrics, "metric", nil, "metric name to allowlist (repeatable)")
	return cmd
}

func observabilityAddonHealthCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "addon-health",
		Short: "Show observability addon health across managed clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			health, err := mgr.ListAddonHealth(context.Background())
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(health, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(health) == 0 {
				fmt.Println("No observability addons found")
				return nil
			}
			fmt.Printf("%-25s %-12s %s\n", "CLUSTER", "AVAILABLE", "DEGRADED")
			for _, h := range health {
				fmt.Printf("%-25s %-12v %v\n", h.Cluster, h.Available, h.Degraded)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func observabilityRetentionCmd() *cobra.Command {
	var retention, blockDuration, deleteDelay string

	cmd := &cobra.Command{
		Use:   "configure-retention",
		Short: "Configure Thanos retention settings on MCO",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := observability.New(c, cfg, logger)
			opts := observability.RetentionOpts{
				RetentionInLocal: retention,
				BlockDuration:    blockDuration,
				DeleteDelay:      deleteDelay,
			}
			fmt.Println("Configuring retention...")
			if err := mgr.ConfigureRetention(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("Retention configured.")
			return nil
		},
	}
	cmd.Flags().StringVar(&retention, "retention", "", "local retention duration (e.g. 24h)")
	cmd.Flags().StringVar(&blockDuration, "block-duration", "", "TSDB block duration (e.g. 2h)")
	cmd.Flags().StringVar(&deleteDelay, "delete-delay", "", "deletion delay for blocks (e.g. 48h)")
	return cmd
}
