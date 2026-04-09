package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"llmsnare/internal/benchmark"
	"llmsnare/internal/config"

	"github.com/spf13/cobra"
)

func newRunCommand() *cobra.Command {
	var asJSON bool
	var casePath string
	var fixtureDir string

	cmd := &cobra.Command{
		Use:   "run [profile_name]",
		Short: "Run the benchmark once",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			caseDef, err := loadCase(cfg, casePath, fixtureDir)
			if err != nil {
				return err
			}

			profiles, err := selectProfiles(cfg, args)
			if err != nil {
				return err
			}

			runner := benchmark.NewRunner()
			results := make([]benchmark.Result, 0, len(profiles))
			for _, namedProfile := range profiles {
				result, runErr := runner.Run(cmd.Context(), caseDef, namedProfile.Name, namedProfile.Profile)
				if runErr != nil {
					return runErr
				}
				results = append(results, result)
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if len(results) == 1 {
					return enc.Encode(results[0])
				}
				return enc.Encode(results)
			}

			for i, result := range results {
				if i > 0 {
					fmt.Fprintln(cmd.OutOrStdout())
				}
				renderTextResult(cmd, result)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Print results as JSON")
	cmd.Flags().StringVar(&casePath, "case", "", "Override the benchmark case file path")
	cmd.Flags().StringVar(&fixtureDir, "fixture-dir", "", "Override the benchmark fixture directory")
	return cmd
}

type namedProfile struct {
	Name    string
	Profile config.Profile
}

func selectProfiles(cfg config.Config, args []string) ([]namedProfile, error) {
	if len(args) == 1 {
		profile, ok := cfg.Profiles[args[0]]
		if !ok {
			return nil, fmt.Errorf("profile %q not found", args[0])
		}
		return []namedProfile{{Name: args[0], Profile: profile}}, nil
	}

	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]namedProfile, 0, len(names))
	for _, name := range names {
		result = append(result, namedProfile{Name: name, Profile: cfg.Profiles[name]})
	}
	return result, nil
}

func renderTextResult(cmd *cobra.Command, result benchmark.Result) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "profile: %s\n", result.Profile)
	fmt.Fprintf(out, "case: %s\n", result.CaseID)
	fmt.Fprintf(out, "success: %t\n", result.Success)
	fmt.Fprintf(out, "score: %d\n", result.TotalScore)
	if result.Error != "" {
		fmt.Fprintf(out, "error: %s\n", result.Error)
	}
	if result.Metrics.ReadWriteRatio != nil {
		fmt.Fprintf(out, "read_write_ratio: %.2f\n", *result.Metrics.ReadWriteRatio)
	} else {
		fmt.Fprintln(out, "read_write_ratio: inf")
	}
	if result.Metrics.PreWriteReadCoverage != nil {
		fmt.Fprintf(out, "pre_write_read_coverage: %.2f\n", *result.Metrics.PreWriteReadCoverage)
	}
	fmt.Fprintf(out, "vendor_trap_recovered: %t\n", result.Metrics.VendorTrapRecovered)
	fmt.Fprintf(out, "util_trap_triggered: %t\n", result.Metrics.UtilTrapTriggered)
	if len(result.Deductions) > 0 {
		fmt.Fprintln(out, "deductions:")
		for _, item := range result.Deductions {
			fmt.Fprintf(out, "  %s %d %s\n", item.Name, item.Points, item.Description)
		}
	}
	if len(result.Bonuses) > 0 {
		fmt.Fprintln(out, "bonuses:")
		for _, item := range result.Bonuses {
			fmt.Fprintf(out, "  %s +%d %s\n", item.Name, item.Points, item.Description)
		}
	}
}
