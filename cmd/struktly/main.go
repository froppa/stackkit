package main

import (
	"fmt"
	"os"

	"github.com/froppa/stackkit/internal/struktly/orchestrator"
	"github.com/spf13/cobra"
)

func main() {
	var specPath string
	var policyPath string
	var outDir string

	var rootCmd = &cobra.Command{
		Use:   "struktly",
		Short: "Struktly Code Generation",
		Run: func(cmd *cobra.Command, args []string) {
			if err := orchestrator.Run(specPath, policyPath, outDir); err != nil {
				// We don't necessarily exit 1 if it's a "controlled failure" (e.g. policy violation),
				// but generally if the tool fails to produce the expected output, exit !0 is good for scripts.
				// However, the prompt says "OR a failure report". If we produce a failure report, maybe we exit 0?
				// "FAIL-FIRST IS A FEATURE... DISCARD IT".
				// "If verification fails, the run fails regardless of AI confidence"
				// I will print the error and exit 1 so CI pipelines know it failed.
				fmt.Fprintf(os.Stderr, "Struktly Run Failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Struktly Run Successful.")
		},
	}

	rootCmd.Flags().StringVar(&specPath, "spec", "", "Path to OpenAPI spec")
	rootCmd.Flags().StringVar(&policyPath, "policy", "", "Path to Policy file")
	rootCmd.Flags().StringVar(&outDir, "out", "output", "Output directory")
	rootCmd.MarkFlagRequired("spec")
	rootCmd.MarkFlagRequired("policy")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
