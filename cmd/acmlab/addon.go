package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/addon"
)

func addonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addon",
		Short: "Manage ACM add-on lifecycle and configurations",
	}
	cmd.AddCommand(addonListCmd(), addonGetCmd(), addonConfigureCmd(), addonRemoveConfigCmd(), addonListConfigsCmd())
	return cmd
}

func addonListCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered ClusterManagementAddOns",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := addon.New(c, cfg, logger)
			addons, err := mgr.ListAddOns(context.Background())
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(addons, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(addons) == 0 {
				fmt.Println("No add-ons found")
				return nil
			}

			fmt.Printf("%-30s %-30s %s\n", "NAME", "DISPLAY NAME", "STATUS")
			for _, a := range addons {
				fmt.Printf("%-30s %-30s %s\n", a.Name, a.DisplayName, a.Status)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func addonGetCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show add-on details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := addon.New(c, cfg, logger)
			detail, err := mgr.GetAddOn(context.Background(), args[0])
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(detail, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Name:          %s\n", detail.Name)
			fmt.Printf("Display Name:  %s\n", detail.DisplayName)
			fmt.Printf("Description:   %s\n", detail.Description)
			fmt.Printf("Status:        %s\n", detail.Status)
			if detail.InstallNamespace != "" {
				fmt.Printf("Install NS:    %s\n", detail.InstallNamespace)
			}
			if len(detail.Configs) > 0 {
				fmt.Printf("Configs:       %s\n", strings.Join(detail.Configs, ", "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}

func addonConfigureCmd() *cobra.Command {
	var namespace, installNamespace string
	var setValues []string

	cmd := &cobra.Command{
		Use:   "configure <name>",
		Short: "Create or update an AddOnDeploymentConfig",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			values := map[string]string{}
			for _, s := range setValues {
				parts := strings.SplitN(s, "=", 2)
				if len(parts) == 2 {
					values[parts[0]] = parts[1]
				}
			}

			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := addon.New(c, cfg, logger)
			opts := addon.AddOnConfigOpts{
				Name:             args[0],
				Namespace:        namespace,
				InstallNamespace: installNamespace,
				Values:           values,
			}
			fmt.Printf("Configuring add-on %s...\n", args[0])
			if err := mgr.ConfigureAddOn(context.Background(), opts); err != nil {
				return err
			}
			fmt.Printf("AddOnDeploymentConfig %s created\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: open-cluster-management)")
	cmd.Flags().StringVar(&installNamespace, "install-namespace", "", "Agent install namespace on spoke clusters")
	cmd.Flags().StringSliceVar(&setValues, "set", nil, "Configuration values (key=value, repeatable)")
	return cmd
}

func addonRemoveConfigCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "remove-config <name>",
		Short: "Remove an AddOnDeploymentConfig",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := addon.New(c, cfg, logger)
			if err := mgr.RemoveConfig(context.Background(), args[0], namespace); err != nil {
				return err
			}
			fmt.Printf("AddOnDeploymentConfig %s removed\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace (default: open-cluster-management)")
	return cmd
}

func addonListConfigsCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "list-configs",
		Short: "List AddOnDeploymentConfig resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := addon.New(c, cfg, logger)
			configs, err := mgr.ListConfigs(context.Background())
			if err != nil {
				return err
			}

			if outputJSON {
				data, _ := json.MarshalIndent(configs, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(configs) == 0 {
				fmt.Println("No AddOnDeploymentConfigs found")
				return nil
			}

			fmt.Printf("%-30s %-30s %s\n", "NAME", "NAMESPACE", "STATUS")
			for _, c := range configs {
				fmt.Printf("%-30s %-30s %s\n", c.Name, c.Namespace, c.Status)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	return cmd
}
