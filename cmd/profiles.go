package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newProfilesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "List configured profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			profiles, err := selectProfiles(cfg, nil)
			if err != nil {
				return err
			}
			renderProfileList(cmd.OutOrStdout(), profiles)
			return nil
		},
	}
	return cmd
}

func renderProfileList(out io.Writer, profiles []namedProfile) {
	style := newANSIStyle(out)
	fmt.Fprintln(out, style.header("Available Profiles"))
	if len(profiles) == 0 {
		fmt.Fprintln(out, "- no profiles found")
		return
	}

	for i, item := range profiles {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "- %s\n", style.emphasis(item.Name))
		fmt.Fprintf(out, "  provider: %s\n", item.Profile.Provider)
		fmt.Fprintf(out, "  model: %s\n", item.Profile.Model)
		fmt.Fprintf(out, "  endpoint: %s\n", item.Profile.Endpoint)
	}
}
