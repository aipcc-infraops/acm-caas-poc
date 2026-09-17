package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/security"
)

func securityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "security",
		Short: "Manage security baselines and compliance scanning on managed clusters",
	}
	cmd.AddCommand(
		securityApplyCmd(),
		securityStatusCmd(),
		securityListCmd(),
		securityRemoveCmd(),
		securityApplyPolicyCmd(),
		securityListPoliciesCmd(),
		securityRemovePolicyCmd(),
		securityDeployComplianceCmd(),
		securityScanCmd(),
		securityScanStatusCmd(),
		securityComplianceReportCmd(),
		securityRemoveComplianceCmd(),
	)
	return cmd
}

func securityApplyCmd() *cobra.Command {
	var cluster, clusterSet string

	cmd := &cobra.Command{
		Use:   "apply <level>",
		Short: "Apply a security baseline (e.g., cis-level1) to a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			fmt.Printf("Applying %s security baseline to %s...\n", args[0], cluster)
			if err := mgr.ApplyBaseline(context.Background(), cluster, args[0], clusterSet); err != nil {
				return err
			}
			fmt.Println("Security baseline applied. Gatekeeper constraints deployed via ManifestWork.")
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster name (required)")
	cmd.Flags().StringVar(&clusterSet, "cluster-set", "", "scope health policy to a ClusterSet")
	return cmd
}

func securityStatusCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "status <cluster>",
		Short: "Show security baseline status for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			status, err := mgr.GetStatus(context.Background(), args[0])
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Cluster:  %s\n", status.Cluster)
			fmt.Printf("Level:    %s\n", status.Level)
			fmt.Printf("Applied:  %v\n", status.Applied)
			if len(status.Conditions) > 0 {
				fmt.Println("Conditions:")
				for _, c := range status.Conditions {
					fmt.Printf("  - %s\n", c)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func securityListCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all security baselines across the fleet",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			baselines, err := mgr.ListBaselines(context.Background())
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(baselines, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(baselines) == 0 {
				fmt.Println("No security baselines found")
				return nil
			}
			fmt.Printf("%-25s %-15s %s\n", "CLUSTER", "LEVEL", "STATUS")
			for _, b := range baselines {
				fmt.Printf("%-25s %-15s %s\n", b.Cluster, b.Level, b.Status)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func securityRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <cluster>",
		Short: "Remove security baseline from a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			fmt.Printf("Removing security baseline from %s...\n", args[0])
			removed, err := mgr.RemoveBaseline(context.Background(), args[0])
			if err != nil {
				return err
			}
			if removed {
				fmt.Println("Security baseline removed. All resources cleaned up.")
			} else {
				fmt.Printf("No security baseline found on %s (nothing to remove)\n", args[0])
			}
			return nil
		},
	}
	return cmd
}

func securityApplyPolicyCmd() *cobra.Command {
	var cluster, policyFile, name, engine string
	var matchKinds []string

	cmd := &cobra.Command{
		Use:   "apply-policy",
		Short: "Apply a custom policy (Gatekeeper Rego or Kyverno YAML) to a cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			if policyFile == "" {
				return fmt.Errorf("--policy-file is required")
			}
			if engine == "" {
				if strings.HasSuffix(policyFile, ".rego") {
					engine = "gatekeeper"
				} else {
					engine = "kyverno"
				}
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)

			switch engine {
			case "gatekeeper":
				opts := security.CustomPolicyOpts{
					Name:     name,
					Cluster:  cluster,
					RegoFile: policyFile,
					Match:    matchKinds,
				}
				fmt.Printf("Applying Gatekeeper policy from %s to %s...\n", policyFile, cluster)
				if err := mgr.ApplyCustomPolicy(context.Background(), opts); err != nil {
					return err
				}
				fmt.Println("Gatekeeper policy deployed via ManifestWork.")
			case "kyverno":
				opts := security.KyvernoPolicyOpts{
					Name:       name,
					Cluster:    cluster,
					PolicyFile: policyFile,
				}
				fmt.Printf("Applying Kyverno policy from %s to %s...\n", policyFile, cluster)
				if err := mgr.ApplyKyvernoPolicy(context.Background(), opts); err != nil {
					return err
				}
				fmt.Println("Kyverno ClusterPolicy deployed via ManifestWork.")
			default:
				return fmt.Errorf("unknown engine %q (valid: gatekeeper, kyverno)", engine)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster name (required)")
	cmd.Flags().StringVar(&policyFile, "policy-file", "", "path to policy file (.rego or .yaml)")
	cmd.Flags().StringVar(&name, "name", "", "policy name (auto-detected from file if omitted)")
	cmd.Flags().StringVar(&engine, "engine", "", "policy engine: gatekeeper, kyverno (auto-detected from extension)")
	cmd.Flags().StringSliceVar(&matchKinds, "match", []string{"Pod"}, "Kubernetes kinds to match (Gatekeeper only)")
	return cmd
}

func securityRemovePolicyCmd() *cobra.Command {
	var engine string

	cmd := &cobra.Command{
		Use:   "remove-policy <name> --cluster <cluster>",
		Short: "Remove a custom policy (Gatekeeper or Kyverno) from a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, _ := cmd.Flags().GetString("cluster")
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			if engine == "" {
				return fmt.Errorf("--engine is required (gatekeeper or kyverno)")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)

			switch engine {
			case "gatekeeper":
				if err := mgr.RemoveCustomPolicy(context.Background(), args[0], cluster); err != nil {
					return err
				}
			case "kyverno":
				if err := mgr.RemoveKyvernoPolicy(context.Background(), args[0], cluster); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown engine %q (valid: gatekeeper, kyverno)", engine)
			}
			fmt.Printf("Policy %s removed from %s\n", args[0], cluster)
			return nil
		},
	}
	cmd.Flags().String("cluster", "", "target cluster name (required)")
	cmd.Flags().StringVar(&engine, "engine", "", "policy engine: gatekeeper, kyverno (required)")
	return cmd
}

func securityListPoliciesCmd() *cobra.Command {
	var engine string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list-policies",
		Short: "List custom policies across the fleet (Gatekeeper and/or Kyverno)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)

			all := make([]policyEntry, 0)
			if engine == "" || engine == "gatekeeper" {
				policies, err := mgr.ListCustomPolicies(context.Background())
				if err != nil {
					return err
				}
				for _, p := range policies {
					all = append(all, policyEntry{Engine: "gatekeeper", Info: p})
				}
			}
			if engine == "" || engine == "kyverno" {
				policies, err := mgr.ListKyvernoPolicies(context.Background())
				if err != nil {
					return err
				}
				for _, p := range policies {
					all = append(all, policyEntry{Engine: "kyverno", Info: p})
				}
			}

			if outputJSON {
				data, _ := json.MarshalIndent(all, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(all) == 0 {
				fmt.Println("No custom policies found")
				return nil
			}
			fmt.Printf("%-12s %-25s %-20s %s\n", "ENGINE", "CLUSTER", "POLICY", "STATUS")
			for _, p := range all {
				fmt.Printf("%-12s %-25s %-20s %s\n", p.Engine, p.Info.Cluster, p.Info.Level, p.Info.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&engine, "engine", "", "filter by engine: gatekeeper, kyverno (default: all)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

type policyEntry struct {
	Engine string             `json:"engine"`
	Info   security.BaselineInfo `json:"info"`
}

func securityDeployComplianceCmd() *cobra.Command {
	var cluster, clusterSet string

	cmd := &cobra.Command{
		Use:   "deploy-compliance",
		Short: "Deploy Compliance Operator to a cluster via ACM OperatorPolicy",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			fmt.Printf("Deploying Compliance Operator to %s...\n", cluster)
			if err := mgr.DeployComplianceOperator(context.Background(), cluster, clusterSet); err != nil {
				return err
			}
			fmt.Println("Compliance Operator deployed. Use 'acmlab security scan' to start a scan.")
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster name (required)")
	cmd.Flags().StringVar(&clusterSet, "cluster-set", "", "scope placement to a ClusterSet")
	return cmd
}

func securityScanCmd() *cobra.Command {
	var cluster, profile, clusterSet string

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Create a compliance scan on a cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			opts := security.ComplianceScanOpts{
				Cluster:    cluster,
				Profile:    profile,
				ClusterSet: clusterSet,
			}
			fmt.Printf("Creating compliance scan on %s with profile %s...\n", cluster, opts.Profile)
			if err := mgr.CreateComplianceScan(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("Compliance scan created. Use 'acmlab security scan-status' to check progress.")
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster name (required)")
	cmd.Flags().StringVar(&profile, "profile", "ocp4-cis", "compliance profile (e.g. ocp4-cis, ocp4-moderate)")
	cmd.Flags().StringVar(&clusterSet, "cluster-set", "", "scope placement to a ClusterSet")
	return cmd
}

func securityScanStatusCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "scan-status <cluster>",
		Short: "Show compliance scan status for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			status, err := mgr.GetComplianceStatus(context.Background(), args[0])
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Cluster:       %s\n", status.Cluster)
			fmt.Printf("Profile:       %s\n", status.Profile)
			fmt.Printf("Phase:         %s\n", status.Phase)
			fmt.Printf("Compliant:     %d\n", status.Compliant)
			fmt.Printf("Non-Compliant: %d\n", status.NonCompliant)
			if len(status.Conditions) > 0 {
				fmt.Println("Conditions:")
				for _, c := range status.Conditions {
					fmt.Printf("  - %s\n", c)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func securityComplianceReportCmd() *cobra.Command {
	var profile string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "compliance-report <cluster>",
		Short: "Show detailed compliance check results for a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			report, err := mgr.GetComplianceReport(context.Background(), args[0], profile)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Cluster: %s\n", report.Cluster)
			fmt.Printf("Profile: %s\n", report.Profile)
			fmt.Printf("Phase:   %s\n", report.Summary.Phase)
			fmt.Printf("Pass:    %d  Fail: %d\n\n", report.Summary.Compliant, report.Summary.NonCompliant)
			if len(report.Results) == 0 {
				fmt.Println("No results available yet.")
				return nil
			}
			fmt.Printf("%-40s %-8s %-8s %s\n", "RULE", "STATUS", "SEV", "DETAIL")
			for _, r := range report.Results {
				detail := r.Detail
				if len(detail) > 40 {
					detail = detail[:37] + "..."
				}
				fmt.Printf("%-40s %-8s %-8s %s\n", r.Rule, r.Status, r.Severity, detail)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "ocp4-cis", "compliance profile")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func securityRemoveComplianceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-compliance <cluster>",
		Short: "Remove compliance scanning from a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := security.New(c, cfg, logger)
			fmt.Printf("Removing compliance scanning from %s...\n", args[0])
			if err := mgr.RemoveComplianceScan(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println("Compliance scanning removed.")
			return nil
		},
	}
	return cmd
}
