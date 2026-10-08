package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
		submarinerDiagnoseCmd(),
		submarinerTestConnectivityCmd(),
		submarinerCreateTestSetCmd(),
		submarinerRepairCmd(),
	)
	return cmd
}

func submarinerEnableCmd() *cobra.Command {
	var (
		wait           bool
		timeout        time.Duration
		globalnet      bool
		forceUDPEncaps bool
		loadBalancer   bool
	)

	cmd := &cobra.Command{
		Use:   "enable <cluster-set>",
		Short: "Enable Submariner connectivity for a ClusterSet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := submariner.New(c, cfg, logger)

			opts := submariner.EnableOpts{
				Globalnet:      globalnet,
				ForceUDPEncaps: forceUDPEncaps,
				LoadBalancer:   loadBalancer,
			}
			if globalnet {
				fmt.Printf("Enabling Submariner with Globalnet for ClusterSet %s...\n", args[0])
			} else {
				fmt.Printf("Enabling Submariner for ClusterSet %s...\n", args[0])
			}
			if err := mgr.Enable(context.Background(), args[0], opts); err != nil {
				return err
			}
			fmt.Println("Submariner enabled. ManagedClusterAddOn + SubmarinerConfig created for all clusters in set.")

			if !wait {
				return nil
			}

			fmt.Printf("Waiting up to %s for Submariner readiness...\n", timeout)
			status, err := mgr.WaitForReady(context.Background(), args[0], timeout)
			if err != nil {
				if status != nil {
					for _, cs := range status.Clusters {
						state := "ready"
						if !cs.GatewayReady || !cs.AgentReady || cs.ConnectionDegraded || cs.Connections == 0 {
							state = "not ready"
						}
						fmt.Printf("  %s: %s\n", cs.Name, state)
					}
				}
				return err
			}
			fmt.Println("Submariner is connected and ready.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for Submariner to become fully connected")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Timeout when waiting for readiness")
	cmd.Flags().BoolVar(&globalnet, "globalnet", false, "Enable Globalnet for overlapping Pod/Service CIDRs")
	cmd.Flags().BoolVar(&forceUDPEncaps, "force-udp-encaps", false, "Force UDP encapsulation for NAT traversal")
	cmd.Flags().BoolVar(&loadBalancer, "load-balancer", false, "Enable LoadBalancer for gateway (not supported on IBM Cloud UDP)")
	return cmd
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
			fmt.Fprintf(cmd.OutOrStdout(), "ClusterSet: %s  Status: %s\n", status.ClusterSet, conn)
			for _, cs := range status.Clusters {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", cs.Name)
				fmt.Fprintf(cmd.OutOrStdout(), "  addonAvailable: %t\n", cs.AddonAvailable)
				fmt.Fprintf(cmd.OutOrStdout(), "  gatewayReady: %t\n", cs.GatewayReady)
				fmt.Fprintf(cmd.OutOrStdout(), "  agentReady: %t\n", cs.AgentReady)
				fmt.Fprintf(cmd.OutOrStdout(), "  connections: %d\n", cs.Connections)
				fmt.Fprintf(cmd.OutOrStdout(), "  connectionDegraded: %t\n", cs.ConnectionDegraded)
				if cs.Reason != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  reason: %s\n", cs.Reason)
				}
				if cs.Message != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  message: %s\n", cs.Message)
				}
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

func submarinerDiagnoseCmd() *cobra.Command {
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "diagnose <cluster-set>",
		Short: "Diagnose Submariner connectivity issues for a ClusterSet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := submariner.New(c, cfg, logger)
			result, err := mgr.Diagnose(context.Background(), args[0])
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			for _, check := range result.Checks {
				var icon string
				switch check.Status {
				case "pass":
					icon = "[ok]"
				case "fail":
					icon = "[FAIL]"
				case "warn":
					icon = "[WARN]"
				case "info":
					icon = "[INFO]"
				default:
					icon = "[skip]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %-8s %-30s %s\n", icon, check.Name, check.Message)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			if result.Healthy {
				fmt.Fprintln(cmd.OutOrStdout(), "Submariner connectivity looks healthy.")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Issues detected. Review recommendations above.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "JSON output")
	return cmd
}

func submarinerTestConnectivityCmd() *cobra.Command {
	var (
		jsonFlag    bool
		namespace   string
		timeout     time.Duration
		cleanup     bool
		image       string
		serverImage string
		clientImage string
		kubeconfigA string
		kubeconfigB string
	)

	cmd := &cobra.Command{
		Use:   "test-connectivity <cluster-a> <cluster-b>",
		Short: "Test cross-cluster connectivity via Submariner",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			si, ci := serverImage, clientImage
			if image != "" {
				if si == "" {
					si = image
				}
				if ci == "" {
					ci = image
				}
			}
			if si == "" {
				si = submariner.DefaultServerImage
			}
			if ci == "" {
				ci = submariner.DefaultClientImage
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := submariner.New(c, cfg, logger)
			result, err := mgr.TestConnectivity(context.Background(), submariner.ConnectivityTestOpts{
				ClusterA:    args[0],
				ClusterB:    args[1],
				Namespace:   namespace,
				Timeout:     timeout,
				Cleanup:     cleanup,
				ServerImage: si,
				ClientImage: ci,
				KubeconfigA: kubeconfigA,
				KubeconfigB: kubeconfigB,
			})
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			status := "FAIL"
			if result.Success {
				status = "PASS"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Connectivity Test: %s\n", status)
			fmt.Fprintf(cmd.OutOrStdout(), "  clusterA: %s\n", result.ClusterA)
			fmt.Fprintf(cmd.OutOrStdout(), "  clusterB: %s\n", result.ClusterB)
			fmt.Fprintf(cmd.OutOrStdout(), "  phase: %s\n", result.Phase)
			fmt.Fprintf(cmd.OutOrStdout(), "  message: %s\n", result.Message)
			if result.Details != nil {
				if result.Details.ServerPodPhase != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  serverPod: phase=%s reason=%s\n", result.Details.ServerPodPhase, result.Details.ServerPodReason)
				}
				if result.Details.ClientPodPhase != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  clientPod: phase=%s reason=%s\n", result.Details.ClientPodPhase, result.Details.ClientPodReason)
				}
			}
			if !cleanup {
				fmt.Fprintf(cmd.OutOrStdout(), "\nCleanup disabled. Resources left in namespace %q on both clusters.\n", namespace)
				fmt.Fprintf(cmd.OutOrStdout(), "To clean up manually:\n")
				fmt.Fprintf(cmd.OutOrStdout(), "  kubectl --kubeconfig=<A> delete ns %s\n", namespace)
				fmt.Fprintf(cmd.OutOrStdout(), "  kubectl --kubeconfig=<B> delete ns %s\n", namespace)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "JSON output")
	cmd.Flags().StringVar(&namespace, "namespace", "acmlab-submariner-test", "test namespace")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "timeout for connectivity test")
	cmd.Flags().BoolVar(&cleanup, "cleanup", true, "clean up test resources after test")
	cmd.Flags().StringVar(&image, "image", "", "container image for both server and client pods")
	cmd.Flags().StringVar(&serverImage, "server-image", "", "container image for server pod (overrides --image)")
	cmd.Flags().StringVar(&clientImage, "client-image", "", "container image for client pod (overrides --image)")
	cmd.Flags().StringVar(&kubeconfigA, "kubeconfig-a", "", "kubeconfig path for cluster A")
	cmd.Flags().StringVar(&kubeconfigB, "kubeconfig-b", "", "kubeconfig path for cluster B")
	return cmd
}

func submarinerRepairCmd() *cobra.Command {
	var (
		jsonFlag bool
		dryRun   bool
	)

	cmd := &cobra.Command{
		Use:   "repair <cluster-set>",
		Short: "Detect and repair stuck Submariner state for a ClusterSet",
		Long:  "Detects stuck addon finalizers, missing Broker CR, and duplicate ManagedClusterSet finalizers. Use --dry-run to preview without changes.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := submariner.New(c, cfg, logger)
			result, err := mgr.Repair(context.Background(), args[0], dryRun)
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			if len(result.Actions) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No issues found.")
				return nil
			}
			for _, a := range result.Actions {
				var icon string
				switch a.Status {
				case "fixed":
					icon = "[FIXED]"
				case "ok":
					icon = "[ok]"
				case "would-fix":
					icon = "[DRY-RUN]"
				case "failed":
					icon = "[FAILED]"
				default:
					icon = "[" + a.Status + "]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %-12s %-30s %s\n", icon, a.Name, a.Message)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "JSON output")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview repairs without making changes")
	return cmd
}

func submarinerCreateTestSetCmd() *cobra.Command {
	var (
		clusters string
		confirm  bool
	)

	cmd := &cobra.Command{
		Use:   "create-test-set <name>",
		Short: "Create a dedicated ClusterSet for Submariner testing",
		Long:  "Creates a ManagedClusterSet and moves the specified clusters into it. Warning: this changes cluster ClusterSet membership, which may affect existing placements.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterList := strings.Split(clusters, ",")
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := submariner.New(c, cfg, logger)
			if err := mgr.CreateTestSet(context.Background(), submariner.CreateTestSetOpts{
				Name:     args[0],
				Clusters: clusterList,
				Confirm:  confirm,
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ClusterSet %q created with clusters: %s\n", args[0], clusters)
			fmt.Fprintf(cmd.OutOrStdout(), "Run 'acmlab submariner enable %s' to deploy Submariner.\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&clusters, "clusters", "", "comma-separated list of cluster names (required)")
	_ = cmd.MarkFlagRequired("clusters")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm cluster relabelling (required)")
	return cmd
}
