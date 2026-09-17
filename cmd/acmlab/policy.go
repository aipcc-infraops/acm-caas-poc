package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/automation"
	"github.com/pablofelix/acm-caas-poc/internal/policy"
)

func policyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Manage governance policies on managed clusters",
	}
	cmd.AddCommand(
		policyListCmd(),
		policyStatusCmd(),
		policyApplyCmd(),
		policyRemoveCmd(),
		policySetRemediationCmd(),
		policyEnableCmd(),
		policyDisableCmd(),
		policyReportCmd(),
		policyApplyQuotaCmd(),
		policyQuotaStatusCmd(),
		policyAutomateCmd(),
		policyAutomationStatusCmd(),
		policyListAutomationsCmd(),
		policyRemoveAutomationCmd(),
		policySetAutomationModeCmd(),
		policyApplySetCmd(),
		policyGetSetCmd(),
		policyListSetsCmd(),
		policyRemoveSetCmd(),
		policyViolationsCmd(),
		policyTroubleshootCmd(),
	)
	return cmd
}

func policyListCmd() *cobra.Command {
	var namespace string
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List policies and compliance status",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			policies, err := mgr.List(context.Background(), namespace)
			if err != nil {
				return err
			}
			if len(policies) == 0 {
				fmt.Println("No policies found")
				return nil
			}
			if outputJSON {
				data, _ := json.MarshalIndent(policies, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("%-35s %-12s %-10s %-15s\n", "NAME", "REMEDIATION", "DISABLED", "COMPLIANT")
			for _, p := range policies {
				compliant := p.Compliant
				if compliant == "" {
					compliant = "-"
				}
				fmt.Printf("%-35s %-12s %-10v %-15s\n", p.Name, p.RemediationAction, p.Disabled, compliant)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func policyStatusCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "status <name>",
		Short: "Show detailed policy compliance status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			info, err := mgr.Get(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			fmt.Printf("Policy:      %s\n", info.Name)
			fmt.Printf("Namespace:   %s\n", info.Namespace)
			fmt.Printf("Remediation: %s\n", info.RemediationAction)
			fmt.Printf("Disabled:    %v\n", info.Disabled)
			compliant := info.Compliant
			if compliant == "" {
				compliant = "Unknown"
			}
			fmt.Printf("Compliant:   %s\n", compliant)
			fmt.Println()
			if len(info.ClusterCompliance) > 0 {
				fmt.Printf("%-30s %s\n", "CLUSTER", "STATE")
				for _, cc := range info.ClusterCompliance {
					fmt.Printf("%-30s %s\n", cc.ClusterName, cc.ComplianceState)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	return cmd
}

func policyApplyCmd() *cobra.Command {
	var namespace, remediation, labels, registries string
	var operatorName, operatorVersion, operatorChannel string
	var certExpiry int
	var certNamespaces string
	cmd := &cobra.Command{
		Use:   "apply <name>",
		Short: "Create a policy with placement (idempotent)",
		Long: `Create a governance policy with placement and binding.

Supports three policy types:
  - ConfigurationPolicy (default): enforce object state (namespaces, registry restrictions)
  - OperatorPolicy: pin operator versions and channels (--operator)
  - CertificatePolicy: detect expiring certificates (--cert-expiry)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			var clusterSet string
			if cs, _ := cmd.Flags().GetString("cluster-set"); cs != "" {
				clusterSet = cs
			}
			opts := policy.PolicyOpts{
				Name:              args[0],
				Namespace:         namespace,
				RemediationAction: remediation,
				OperatorName:      operatorName,
				OperatorVersion:   operatorVersion,
				OperatorChannel:   operatorChannel,
				CertExpiryDays:    certExpiry,
				ClusterSet:        clusterSet,
			}
			if labels != "" {
				opts.ClusterLabels = parseLabels(labels)
			}
			if registries != "" {
				opts.AllowedRegistries = strings.Split(registries, ",")
			}
			if certNamespaces != "" {
				opts.CertNamespaces = strings.Split(certNamespaces, ",")
			}
			fmt.Printf("Applying policy %s...\n", args[0])
			if err := mgr.Apply(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("Policy applied. Use 'acmlab policy status' to check compliance.")
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	cmd.Flags().StringVarP(&remediation, "remediation", "r", "inform", "remediation action: inform or enforce")
	cmd.Flags().StringVarP(&labels, "labels", "l", "", "cluster label selector (key=value,key2=value2)")
	cmd.Flags().StringVar(&registries, "registries", "", "allowed registries (comma-separated)")
	cmd.Flags().StringVar(&operatorName, "operator", "", "operator name for OperatorPolicy (UC-27)")
	cmd.Flags().StringVar(&operatorVersion, "operator-version", "", "pin operator to this version")
	cmd.Flags().StringVar(&operatorChannel, "operator-channel", "", "pin operator to this channel")
	cmd.Flags().IntVar(&certExpiry, "cert-expiry", 0, "certificate expiry threshold in days for CertificatePolicy (UC-28)")
	cmd.Flags().StringVar(&certNamespaces, "cert-namespaces", "", "namespaces to monitor for cert expiry (comma-separated, default: openshift-config,openshift-ingress)")
	cmd.Flags().String("cluster-set", "", "scope policy to a ClusterSet (UC-24)")
	return cmd
}

func policyRemoveCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a policy and its placement — clean, no leftovers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			fmt.Printf("Removing policy %s...\n", args[0])
			removed, err := mgr.Remove(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if removed {
				fmt.Println("Policy removed. All resources cleaned up.")
			} else {
				fmt.Printf("Policy %s not found (nothing to remove)\n", args[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	return cmd
}

func policySetRemediationCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "set-remediation <name> <inform|enforce>",
		Short: "Change policy remediation action",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			action := args[1]
			if action != "inform" && action != "enforce" {
				return fmt.Errorf("invalid remediation action %q: must be 'inform' or 'enforce'", action)
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			if err := mgr.SetRemediation(context.Background(), args[0], namespace, action); err != nil {
				return err
			}
			fmt.Printf("Policy %s remediation set to %s\n", args[0], args[1])
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	return cmd
}

func policyEnableCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a disabled policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			if err := mgr.SetDisabled(context.Background(), args[0], namespace, false); err != nil {
				return err
			}
			fmt.Printf("Policy %s enabled\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	return cmd
}

func policyDisableCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a policy without removing it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			if err := mgr.SetDisabled(context.Background(), args[0], namespace, true); err != nil {
				return err
			}
			fmt.Printf("Policy %s disabled\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	return cmd
}

func policyReportCmd() *cobra.Command {
	var namespace string
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Show per-ClusterSet compliance report",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			report, err := mgr.ComplianceReport(context.Background(), namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(report) == 0 {
				fmt.Println("No compliance data found")
				return nil
			}
			fmt.Printf("%-25s %-8s %-10s %-12s %s\n", "CLUSTERSET", "TOTAL", "COMPLIANT", "NONCOMPLIANT", "PENDING")
			for _, r := range report {
				fmt.Printf("%-25s %-8d %-10d %-12d %d\n", r.ClusterSet, r.Total, r.Compliant, r.NonCompliant, r.Pending)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func policyApplyQuotaCmd() *cobra.Command {
	var cluster string
	var maxWorkers, maxGPUs int
	cmd := &cobra.Command{
		Use:   "apply-quota",
		Short: "Apply resource quota policy to a cluster (UC-15)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			if maxWorkers <= 0 && maxGPUs <= 0 {
				return fmt.Errorf("at least one of --max-workers or --max-gpus must be positive")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			fmt.Printf("Applying quota policy to %s (max-workers=%d, max-gpus=%d)...\n", cluster, maxWorkers, maxGPUs)
			if err := mgr.ApplyQuotaPolicy(context.Background(), cluster, maxWorkers, maxGPUs); err != nil {
				return err
			}
			fmt.Println("Quota policy applied. Use 'acmlab policy quota-status' to check compliance.")
			return nil
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "target cluster name (required)")
	cmd.Flags().IntVar(&maxWorkers, "max-workers", 0, "maximum worker nodes allowed")
	cmd.Flags().IntVar(&maxGPUs, "max-gpus", 0, "maximum GPU nodes allowed")
	return cmd
}

func policyQuotaStatusCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "quota-status <cluster>",
		Short: "Show quota compliance for a cluster (UC-15)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			status, err := mgr.GetQuotaStatus(context.Background(), args[0])
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(status, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Cluster:      %s\n", status.Cluster)
			fmt.Printf("Max Workers:  %d\n", status.MaxWorkers)
			fmt.Printf("Max GPUs:     %d\n", status.MaxGPUs)
			fmt.Printf("Policy:       %s\n", status.PolicyName)
			fmt.Printf("Compliant:    %s\n", status.Compliant)
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func parseLabels(s string) map[string]string {
	labels := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			labels[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return labels
}

func policyAutomateCmd() *cobra.Command {
	var policyName, towerURL, towerSecret, jobTemplate, mode, namespace string
	var extraVars []string

	cmd := &cobra.Command{
		Use:   "automate <name>",
		Short: "Create a PolicyAutomation linking a policy to an Ansible job template (UC-30)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if policyName == "" {
				return fmt.Errorf("--policy is required")
			}
			if towerSecret == "" {
				return fmt.Errorf("--tower-secret is required")
			}
			if jobTemplate == "" {
				return fmt.Errorf("--job-template is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := automation.New(c, cfg, logger)

			vars := map[string]string{}
			for _, ev := range extraVars {
				parts := strings.SplitN(ev, "=", 2)
				if len(parts) == 2 {
					vars[parts[0]] = parts[1]
				}
			}

			opts := automation.AutomationOpts{
				Name:        args[0],
				Namespace:   namespace,
				PolicyName:  policyName,
				Mode:        mode,
				TowerURL:    towerURL,
				TowerSecret: towerSecret,
				JobTemplate: jobTemplate,
				ExtraVars:   vars,
			}

			fmt.Printf("Creating PolicyAutomation %s for policy %s...\n", args[0], policyName)
			if err := mgr.Create(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("PolicyAutomation created. Ansible jobs will trigger on policy violations.")
			return nil
		},
	}
	cmd.Flags().StringVar(&policyName, "policy", "", "referenced policy name (required)")
	cmd.Flags().StringVar(&towerURL, "tower-url", "", "Ansible Tower URL")
	cmd.Flags().StringVar(&towerSecret, "tower-secret", "", "secret name with tower credentials (required)")
	cmd.Flags().StringVar(&jobTemplate, "job-template", "", "Ansible job template name (required)")
	cmd.Flags().StringVar(&mode, "mode", "scan", "automation mode: scan, once, disabled")
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace (default: open-cluster-management-policies)")
	cmd.Flags().StringArrayVar(&extraVars, "extra-var", nil, "extra variables (key=value)")
	return cmd
}

func policyAutomationStatusCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "automation-status <name>",
		Short: "Show PolicyAutomation status (UC-30)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := automation.New(c, cfg, logger)
			info, err := mgr.Get(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Name:       %s\n", info.Name)
			fmt.Printf("Namespace:  %s\n", info.Namespace)
			fmt.Printf("Policy:     %s\n", info.PolicyName)
			fmt.Printf("Mode:       %s\n", info.Mode)
			fmt.Printf("Status:     %s\n", info.Status)
			if info.LastRun != "" {
				fmt.Printf("Last Run:   %s\n", info.LastRun)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace (default: open-cluster-management-policies)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func policyListAutomationsCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list-automations",
		Short: "List all PolicyAutomations (UC-30)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := automation.New(c, cfg, logger)
			infos, err := mgr.List(context.Background(), namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(infos, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(infos) == 0 {
				fmt.Println("No policy automations found")
				return nil
			}
			fmt.Printf("%-25s %-25s %-10s %s\n", "NAME", "POLICY", "MODE", "STATUS")
			for _, info := range infos {
				fmt.Printf("%-25s %-25s %-10s %s\n", info.Name, info.PolicyName, info.Mode, info.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace (default: open-cluster-management-policies)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func policyRemoveAutomationCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "remove-automation <name>",
		Short: "Remove a PolicyAutomation (UC-30)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := automation.New(c, cfg, logger)
			fmt.Printf("Removing PolicyAutomation %s...\n", args[0])
			removed, err := mgr.Delete(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if removed {
				fmt.Println("PolicyAutomation removed.")
			} else {
				fmt.Printf("PolicyAutomation %s not found (nothing to remove)\n", args[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace (default: open-cluster-management-policies)")
	return cmd
}

func policySetAutomationModeCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "set-automation-mode <name> <mode>",
		Short: "Update the automation mode: scan, once, disabled (UC-30)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := automation.New(c, cfg, logger)
			if err := mgr.UpdateMode(context.Background(), args[0], namespace, args[1]); err != nil {
				return err
			}
			fmt.Printf("PolicyAutomation %s mode set to %s\n", args[0], args[1])
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace (default: open-cluster-management-policies)")
	return cmd
}

func policyApplySetCmd() *cobra.Command {
	var namespace, description, clusterSet string
	var policies []string

	cmd := &cobra.Command{
		Use:   "apply-set <name>",
		Short: "Create a PolicySet compliance profile (UC-49)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(policies) == 0 {
				return fmt.Errorf("--policies is required")
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			opts := policy.PolicySetOpts{
				Name:        args[0],
				Namespace:   namespace,
				Description: description,
				Policies:    policies,
				ClusterSet:  clusterSet,
			}
			fmt.Printf("Creating PolicySet %s...\n", args[0])
			if err := mgr.ApplyPolicySet(context.Background(), opts); err != nil {
				return err
			}
			fmt.Println("PolicySet created. Grouped policies will be evaluated as a compliance profile.")
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	cmd.Flags().StringVar(&description, "description", "", "PolicySet description")
	cmd.Flags().StringSliceVar(&policies, "policies", nil, "policy names to include (required)")
	cmd.Flags().StringVar(&clusterSet, "cluster-set", "", "scope to a ClusterSet")
	return cmd
}

func policyGetSetCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "get-set <name>",
		Short: "Show PolicySet details (UC-49)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			info, err := mgr.GetPolicySet(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(info, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("PolicySet:   %s\n", info.Name)
			fmt.Printf("Namespace:   %s\n", info.Namespace)
			fmt.Printf("Description: %s\n", info.Description)
			compliant := info.Compliant
			if compliant == "" {
				compliant = "Unknown"
			}
			fmt.Printf("Compliant:   %s\n", compliant)
			fmt.Println("\nPolicies:")
			for _, p := range info.Policies {
				fmt.Printf("  - %s\n", p)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func policyListSetsCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list-sets",
		Short: "List all PolicySets (UC-49)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			sets, err := mgr.ListPolicySets(context.Background(), namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(sets, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(sets) == 0 {
				fmt.Println("No PolicySets found")
				return nil
			}
			fmt.Printf("%-25s %-15s %-10s %s\n", "NAME", "COMPLIANT", "POLICIES", "DESCRIPTION")
			for _, s := range sets {
				compliant := s.Compliant
				if compliant == "" {
					compliant = "-"
				}
				desc := s.Description
				if len(desc) > 40 {
					desc = desc[:37] + "..."
				}
				fmt.Printf("%-25s %-15s %-10d %s\n", s.Name, compliant, len(s.Policies), desc)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func policyRemoveSetCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "remove-set <name>",
		Short: "Remove a PolicySet and its placement (UC-49)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			fmt.Printf("Removing PolicySet %s...\n", args[0])
			removed, err := mgr.RemovePolicySet(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if removed {
				fmt.Println("PolicySet removed. All resources cleaned up.")
			} else {
				fmt.Printf("PolicySet %s not found (nothing to remove)\n", args[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	return cmd
}

func policyViolationsCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "violations <name>",
		Short: "Show policy violations per cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			violations, err := mgr.GetViolations(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(violations, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Policy:    %s\n", violations.Name)
			fmt.Printf("Compliant: %s\n", violations.Compliant)
			if len(violations.Violations) == 0 {
				fmt.Println("\nNo violations found")
				return nil
			}
			fmt.Printf("\n%-25s %s\n", "CLUSTER", "MESSAGE")
			for _, v := range violations.Violations {
				msg := v.Message
				if len(msg) > 60 {
					msg = msg[:57] + "..."
				}
				fmt.Printf("%-25s %s\n", v.Cluster, msg)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func policyTroubleshootCmd() *cobra.Command {
	var namespace string
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "troubleshoot <name>",
		Short: "Troubleshoot a policy: violations + events + propagation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := policy.New(c, cfg, logger)
			report, err := mgr.Troubleshoot(context.Background(), args[0], namespace)
			if err != nil {
				return err
			}
			if outputJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("Policy:    %s\n", report.Policy)
			fmt.Printf("Namespace: %s\n", report.Namespace)
			fmt.Printf("Compliant: %s\n", report.Compliant)

			if len(report.Violations) > 0 {
				fmt.Printf("\nViolations (%d):\n", len(report.Violations))
				for _, v := range report.Violations {
					fmt.Printf("  - %s: %s\n", v.Cluster, v.Message)
				}
			} else {
				fmt.Println("\nNo violations")
			}

			if len(report.Events) > 0 {
				fmt.Printf("\nRecent Events (%d):\n", len(report.Events))
				for _, e := range report.Events {
					fmt.Printf("  [%s] %s: %s (%s)\n", e.Type, e.Reason, e.Message, e.Timestamp)
				}
			} else {
				fmt.Println("\nNo recent events in policy namespace")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "policy namespace (default: global-set)")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
