package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func contextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Switch kubeconfig context between hub and spoke clusters",
	}
	cmd.AddCommand(
		contextCurrentCmd(),
		contextHubCmd(),
		contextSpokeCmd(),
		contextListCmd(),
	)
	return cmd
}

func kubeconfigPath() string {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	return rules.GetDefaultFilename()
}

func contextCurrentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show the current kubeconfig context and whether it is the hub",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := kubeconfigPath()
			kubeCfg, err := clientcmd.LoadFromFile(path)
			if err != nil {
				return fmt.Errorf("loading kubeconfig: %w", err)
			}

			current := kubeCfg.CurrentContext
			if current == "" {
				fmt.Println("No current context set")
				return nil
			}

			ctxVal, ok := kubeCfg.Contexts[current]
			if !ok {
				fmt.Printf("Context:  %s\n", current)
				fmt.Println("Status:   context entry not found in kubeconfig")
				return nil
			}

			server := ""
			if ci, ok := kubeCfg.Clusters[ctxVal.Cluster]; ok {
				server = ci.Server
			}

			contextType := "spoke"
			c, cErr := buildClient()
			if cErr == nil {
				_, listErr := c.Dynamic.Resource(client.GVRManagedCluster).List(
					context.Background(), metav1.ListOptions{Limit: 1},
				)
				if listErr == nil {
					contextType = "hub"
				}
			}

			fmt.Printf("Context:  %s\n", current)
			if server != "" {
				fmt.Printf("Server:   %s\n", server)
			}
			if ctxVal.Namespace != "" {
				fmt.Printf("Namespace: %s\n", ctxVal.Namespace)
			}
			fmt.Printf("Type:     %s\n", contextType)
			return nil
		},
	}
}

func contextHubCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hub",
		Short: "Switch kubeconfig current-context to the ACM hub",
		Long:  "Switches to the hub context. Set ACM_HUB_CONTEXT or use --context to specify the hub context name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			hubCtx := cfg.HubContext
			if hubCtx == "" {
				return fmt.Errorf("hub context not configured; set ACM_HUB_CONTEXT env var or pass --context flag")
			}

			path := kubeconfigPath()
			kubeCfg, err := clientcmd.LoadFromFile(path)
			if err != nil {
				return fmt.Errorf("loading kubeconfig: %w", err)
			}

			if _, ok := kubeCfg.Contexts[hubCtx]; !ok {
				return fmt.Errorf("hub context %q not found in kubeconfig", hubCtx)
			}

			kubeCfg.CurrentContext = hubCtx
			if err := clientcmd.WriteToFile(*kubeCfg, path); err != nil {
				return fmt.Errorf("writing kubeconfig: %w", err)
			}

			fmt.Printf("Switched to hub context: %s\n", hubCtx)
			return nil
		},
	}
}

func contextSpokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "spoke <cluster-name>",
		Short: "Switch kubeconfig current-context to a managed spoke cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]

			c, err := buildClient()
			if err != nil {
				return fmt.Errorf("connecting to hub: %w", err)
			}

			mc, err := c.Dynamic.Resource(client.GVRManagedCluster).Get(
				cmd.Context(), clusterName, metav1.GetOptions{},
			)
			if err != nil {
				return fmt.Errorf("ManagedCluster %q not found on hub: %w", clusterName, err)
			}

			apiURL, _, _ := unstructured.NestedString(mc.Object, "status", "apiServerURL")
			if apiURL == "" {
				configs, _, _ := unstructured.NestedSlice(mc.Object, "spec", "managedClusterClientConfigs")
				for _, c := range configs {
					if m, ok := c.(map[string]interface{}); ok {
						if u, ok := m["url"].(string); ok && u != "" {
							apiURL = u
							break
						}
					}
				}
			}
			if apiURL == "" {
				return fmt.Errorf("no API URL found for ManagedCluster %q; cluster may not have joined yet", clusterName)
			}

			path := kubeconfigPath()
			kubeCfg, err := clientcmd.LoadFromFile(path)
			if err != nil {
				return fmt.Errorf("loading kubeconfig: %w", err)
			}

			normalizedAPI := normalizeURL(apiURL)
			var matchedCtx string
			for ctxName, ctxVal := range kubeCfg.Contexts {
				ci, ok := kubeCfg.Clusters[ctxVal.Cluster]
				if !ok {
					continue
				}
				if normalizeURL(ci.Server) == normalizedAPI {
					matchedCtx = ctxName
					break
				}
			}

			if matchedCtx == "" {
				return fmt.Errorf("no kubeconfig context found for %s (API: %s); log in to the cluster first", clusterName, apiURL)
			}

			kubeCfg.CurrentContext = matchedCtx
			if err := clientcmd.WriteToFile(*kubeCfg, path); err != nil {
				return fmt.Errorf("writing kubeconfig: %w", err)
			}

			fmt.Printf("Switched to spoke context: %s\n", matchedCtx)
			fmt.Printf("Cluster:  %s\n", clusterName)
			fmt.Printf("Server:   %s\n", apiURL)
			return nil
		},
	}
}

func contextListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List kubeconfig contexts with hub/spoke markers",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := kubeconfigPath()
			kubeCfg, err := clientcmd.LoadFromFile(path)
			if err != nil {
				return fmt.Errorf("loading kubeconfig: %w", err)
			}

			spokeURLs := map[string]string{}
			c, hubErr := buildClient()
			if hubErr == nil {
				list, err := c.Dynamic.Resource(client.GVRManagedCluster).List(
					cmd.Context(), metav1.ListOptions{},
				)
				if err == nil {
					for _, item := range list.Items {
						name := item.GetName()
						url, _, _ := unstructured.NestedString(item.Object, "status", "apiServerURL")
						if url != "" {
							spokeURLs[normalizeURL(url)] = name
						}
					}
				}
			}

			hubCtx := cfg.HubContext

			type entry struct {
				marker  string
				ctxName string
				server  string
				ctxType string
			}

			var entries []entry
			for ctxName, ctxVal := range kubeCfg.Contexts {
				server := ""
				if ci, ok := kubeCfg.Clusters[ctxVal.Cluster]; ok {
					server = ci.Server
				}

				marker := " "
				if ctxName == kubeCfg.CurrentContext {
					marker = "*"
				}

				ctxType := "-"
				if hubCtx != "" && ctxName == hubCtx {
					ctxType = "hub"
				} else if server != "" {
					if spokeName, ok := spokeURLs[normalizeURL(server)]; ok {
						ctxType = fmt.Sprintf("spoke (%s)", spokeName)
					}
				}

				entries = append(entries, entry{
					marker:  marker,
					ctxName: ctxName,
					server:  server,
					ctxType: ctxType,
				})
			}

			if len(entries) == 0 {
				fmt.Println("No contexts found in kubeconfig")
				return nil
			}

			fmt.Printf("  %-60s %-55s %s\n", "CONTEXT", "SERVER", "TYPE")
			for _, e := range entries {
				ctx := truncate(e.ctxName, 58)
				srv := truncate(e.server, 53)
				fmt.Printf("%s %-60s %-55s %s\n", e.marker, ctx, srv, e.ctxType)
			}
			return nil
		},
	}
}

func normalizeURL(u string) string {
	u = strings.TrimRight(u, "/")
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	return u
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
