package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablofelix/acm-caas-poc/internal/matrix"
)

func matrixCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "matrix",
		Short: "Multi-architecture cluster matrix provisioning and management (UC-23)",
	}
	cmd.AddCommand(
		matrixProvisionCmd(),
		matrixStatusCmd(),
		matrixDestroyCmd(),
		matrixListCmd(),
	)
	return cmd
}

func matrixProvisionCmd() *cobra.Command {
	var (
		versions  string
		archs     string
		operators string
	)

	cmd := &cobra.Command{
		Use:   "provision",
		Short: "Provision a matrix of clusters for all version/arch/operator combinations",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := matrix.New(c, cfg, logger)
			spec := matrix.MatrixSpec{
				OCPVersions:      strings.Split(versions, ","),
				Architectures:    strings.Split(archs, ","),
				OperatorVersions: strings.Split(operators, ","),
			}
			fmt.Printf("Provisioning matrix: %d versions x %d archs x %d operators...\n",
				len(spec.OCPVersions), len(spec.Architectures), len(spec.OperatorVersions))
			result, err := mgr.ProvisionMatrix(context.Background(), spec)
			if err != nil {
				return err
			}
			if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			fmt.Printf("Matrix provisioned: %d cells (%d ready, %d failed)\n", result.Total, result.Ready, result.Failed)
			for _, cell := range result.Cells {
				fmt.Printf("  - %s (ocp=%s arch=%s op=%s) [%s]\n", cell.Name, cell.OCPVersion, cell.Architecture, cell.OperatorVersion, cell.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&versions, "versions", "", "comma-separated OCP versions (e.g. 4.18,4.19)")
	cmd.Flags().StringVar(&archs, "archs", "", "comma-separated architectures (e.g. amd64,arm64,s390x,ppc64le)")
	cmd.Flags().StringVar(&operators, "operators", "", "comma-separated operator versions (e.g. 2.5,3.0)")
	cmd.Flags().Bool("json", false, "JSON output")
	_ = cmd.MarkFlagRequired("versions")
	_ = cmd.MarkFlagRequired("archs")
	_ = cmd.MarkFlagRequired("operators")
	return cmd
}

func matrixStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <matrix-id>",
		Short: "Show provisioning status for a matrix",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := matrix.New(c, cfg, logger)
			result, err := mgr.StatusMatrix(context.Background(), args[0])
			if err != nil {
				return err
			}
			if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			fmt.Printf("Matrix %s: %d total, %d ready, %d failed\n", args[0], result.Total, result.Ready, result.Failed)
			for _, cell := range result.Cells {
				fmt.Printf("  - %s [%s]\n", cell.Name, cell.Status)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "JSON output")
	return cmd
}

func matrixDestroyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "destroy <matrix-id>",
		Short: "Destroy all clusters in a matrix",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := matrix.New(c, cfg, logger)
			fmt.Printf("Destroying matrix %s...\n", args[0])
			result, err := mgr.DestroyMatrix(context.Background(), args[0])
			if err != nil {
				return err
			}
			if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			fmt.Printf("Matrix %s: %d clusters targeted for deletion (%d failed)\n", args[0], result.Total, result.Failed)
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "JSON output")
	return cmd
}

func matrixListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all matrix clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			mgr := matrix.New(c, cfg, logger)
			cells, err := mgr.ListMatrixClusters(context.Background())
			if err != nil {
				return err
			}
			if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(cells)
			}
			if len(cells) == 0 {
				fmt.Println("No matrix clusters found.")
				return nil
			}
			fmt.Printf("%-35s %-10s %-10s %-15s\n", "NAME", "OCP", "ARCH", "OPERATOR")
			for _, cell := range cells {
				fmt.Printf("%-35s %-10s %-10s %-15s\n", cell.Name, cell.OCPVersion, cell.Architecture, cell.OperatorVersion)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "JSON output")
	return cmd
}
