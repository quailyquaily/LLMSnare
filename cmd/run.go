package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"llmsnare/internal/benchmark"
	"llmsnare/internal/config"
	"llmsnare/internal/storage"

	"github.com/spf13/cobra"
)

func newRunCommand() *cobra.Command {
	var asJSON bool
	var caseRef string
	var persist bool

	cmd := &cobra.Command{
		Use:   "run [profile_name]",
		Short: "Run the benchmark once",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			caseDef, err := loadCase(caseRef)
			if err != nil {
				return err
			}

			profiles, err := selectProfiles(cfg, args)
			if err != nil {
				return err
			}

			results := make([]benchmark.Result, 0, len(profiles))
			for i, namedProfile := range profiles {
				runner := benchmark.NewRunner()
				if reporter := runProgressReporter(cmd, asJSON, i+1, len(profiles)); reporter != nil {
					runner = benchmark.NewRunner(benchmark.WithProgressReporter(reporter))
				}
				result, runErr := runner.Run(cmd.Context(), caseDef, namedProfile.Name, namedProfile.Profile)
				if runErr != nil {
					return runErr
				}
				results = append(results, result)
			}

			if persist {
				if err := persistResults(cfg.Storage.TimelineDir, results); err != nil {
					return err
				}
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
	cmd.Flags().BoolVar(&persist, "persist", false, "Append results to timeline storage")
	cmd.Flags().StringVar(&caseRef, "case", "", "Case ID or case directory path")
	return cmd
}

func persistResults(timelineDir string, results []benchmark.Result) error {
	store := storage.New(timelineDir)
	for _, result := range results {
		if err := store.Append(result); err != nil {
			return err
		}
	}
	return nil
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

func runProgressReporter(cmd *cobra.Command, asJSON bool, profileIndex, totalProfiles int) benchmark.ProgressReporter {
	if asJSON {
		return nil
	}
	return func(event benchmark.ProgressEvent) {
		renderRunProgress(cmd.ErrOrStderr(), profileIndex, totalProfiles, event)
	}
}

func renderTextResult(cmd *cobra.Command, result benchmark.Result) {
	out := cmd.OutOrStdout()
	style := newANSIStyle(out)

	fmt.Fprintf(out, "%s\n", style.dim(strings.Repeat("=", 72)))
	fmt.Fprintf(out, "%s\n", style.header(fmt.Sprintf("Profile: %s", result.Profile)))
	fmt.Fprintf(out, "%s\n", style.dim(strings.Repeat("-", 72)))

	renderKVSection(out, style, "Summary", [][2]string{
		{"Case", result.CaseID},
		{"Status", style.status(formatStatus(result.Success), result.Success)},
		{"Score", style.score(formatPercent(result.NormalizedScore), result.NormalizedScore)},
		{"Raw score", formatRawScore(result.RawScore, result.MaxScore)},
		{"Duration", formatDuration(result.FinishedAt.Sub(result.Timestamp))},
		{"Provider", result.Provider},
		{"Model", result.Model},
		{"Endpoint", result.Endpoint},
		{"Tool calls", fmt.Sprintf("%d", len(result.ToolCalls))},
	})

	if result.Error != "" {
		renderKVSection(out, style, "Error", [][2]string{
			{"Message", style.fail(result.Error)},
		})
	}

	renderKVSection(out, style, "Metrics", [][2]string{
		{style.toolLabel("read_file", "calls"), fmt.Sprintf("%d", result.Metrics.ReadFileCalls)},
		{style.toolLabel("write_file", "calls"), fmt.Sprintf("%d", result.Metrics.WriteFileCalls)},
		{style.toolLabel("list_dir", "calls"), fmt.Sprintf("%d", result.Metrics.ListDirCalls)},
		{"read/write ratio", formatOptionalFloat(result.Metrics.ReadWriteRatio, "inf")},
		{"pre-write read coverage", formatOptionalFloat(result.Metrics.PreWriteReadCoverage, "n/a")},
	})

	renderAdjustmentSection(out, style, "Deductions", result.Deductions)
	renderAdjustmentSection(out, style, "Bonuses", result.Bonuses)
}

func renderRunProgress(out io.Writer, index, total int, event benchmark.ProgressEvent) {
	style := newANSIStyle(out)
	prefix := style.progressPrefix(fmt.Sprintf("[%d/%d]", index, total))

	switch event.Kind {
	case benchmark.ProgressRunStarted:
		fmt.Fprintf(out, "%s %s, profile=%s, case=%s\n", prefix, style.header("started"), event.Profile, event.CaseID)
	case benchmark.ProgressRoundStarted:
		return
	case benchmark.ProgressToolBatch:
		fmt.Fprintf(out, "%s %s %s: received %d tool calls\n", prefix, style.emphasis("round"), formatRound(event.Round), event.ToolCalls)
	case benchmark.ProgressToolExecuted:
		target := style.toolTarget(event.Tool, event.ToolPath)
		if event.Error != "" {
			fmt.Fprintf(out, "%s %s %s: %s, %s: %s\n", prefix, style.emphasis("round"), formatRound(event.Round), target, style.fail("failed"), style.fail(event.Error))
			return
		}
		fmt.Fprintf(out, "%s %s %s: %s\n", prefix, style.emphasis("round"), formatRound(event.Round), target)
	case benchmark.ProgressRunFinished:
		status := style.status(formatStatus(event.Success), event.Success)
		score := style.score(formatPercent(event.NormalizedScore), event.NormalizedScore)
		if event.Error != "" {
			fmt.Fprintf(out, "%s %s, status=%s, elapsed=%s, score=%s, error=%s\n", prefix, style.header("finished"), status, formatDuration(event.Elapsed), score, style.fail(event.Error))
			return
		}
		fmt.Fprintf(out, "%s %s, status=%s, elapsed=%s, score=%s\n", prefix, style.header("finished"), status, formatDuration(event.Elapsed), score)
	}
}

func renderKVSection(out io.Writer, style ansiStyle, title string, rows [][2]string) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(out, "%s\n", style.section(title))
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		fmt.Fprintf(tw, "  %s\t%s\n", row[0], row[1])
	}
	_ = tw.Flush()
	fmt.Fprintln(out)
}

func renderAdjustmentSection(out io.Writer, style ansiStyle, title string, items []benchmark.ScoreAdjustment) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(out, "%s\n", style.section(title))
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, item := range items {
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", style.ruleName(item.Name), style.delta(item.Points), item.Description)
	}
	_ = tw.Flush()
	fmt.Fprintln(out)
}

func formatStatus(success bool) string {
	if success {
		return "PASS"
	}
	return "FAIL"
}

func formatBool(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func formatOptionalFloat(v *float64, fallback string) string {
	if v == nil {
		return fallback
	}
	return fmt.Sprintf("%.2f", *v)
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	return d.Round(time.Millisecond).String()
}

func formatRound(round int) string {
	if round < 0 {
		round = 0
	}
	return fmt.Sprintf("%03d", round)
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.2f%%", value)
}

func formatRawScore(rawScore, maxScore int) string {
	if maxScore <= 0 {
		return fmt.Sprintf("%d", rawScore)
	}
	return fmt.Sprintf("%d/%d", rawScore, maxScore)
}

type ansiStyle struct {
	enabled bool
}

func newANSIStyle(out io.Writer) ansiStyle {
	if os.Getenv("NO_COLOR") != "" {
		return ansiStyle{}
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return ansiStyle{}
	}

	file, ok := out.(*os.File)
	if !ok {
		return ansiStyle{}
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return ansiStyle{}
	}
	return ansiStyle{enabled: true}
}

func (s ansiStyle) wrap(code, text string) string {
	if !s.enabled || text == "" {
		return text
	}
	return code + text + "\x1b[0m"
}

func (s ansiStyle) header(text string) string {
	return s.wrap("\x1b[1;36m", text)
}

func (s ansiStyle) section(text string) string {
	return s.wrap("\x1b[1;34m", text)
}

func (s ansiStyle) emphasis(text string) string {
	return s.wrap("\x1b[1m", text)
}

func (s ansiStyle) dim(text string) string {
	return s.wrap("\x1b[2m", text)
}

func (s ansiStyle) fail(text string) string {
	return s.wrap("\x1b[31m", text)
}

func (s ansiStyle) success(text string) string {
	return s.wrap("\x1b[32m", text)
}

func (s ansiStyle) warn(text string) string {
	return s.wrap("\x1b[33m", text)
}

func (s ansiStyle) progressPrefix(text string) string {
	return s.wrap("\x1b[36m", text)
}

func (s ansiStyle) ruleName(text string) string {
	return s.wrap("\x1b[1m", text)
}

func (s ansiStyle) toolName(text string) string {
	return s.wrap("\x1b[1;35m", text)
}

func (s ansiStyle) toolTarget(name, path string) string {
	if path == "" {
		return s.toolName(name)
	}
	return fmt.Sprintf("%s %s", s.toolName(name), path)
}

func (s ansiStyle) toolLabel(name, suffix string) string {
	if suffix == "" {
		return s.toolName(name)
	}
	return fmt.Sprintf("%s %s", s.toolName(name), suffix)
}

func (s ansiStyle) delta(points int) string {
	text := fmt.Sprintf("%+d", points)
	switch {
	case points > 0:
		return s.success(text)
	case points < 0:
		return s.fail(text)
	default:
		return text
	}
}

func (s ansiStyle) status(text string, success bool) string {
	if success {
		return s.success(text)
	}
	return s.fail(text)
}

func (s ansiStyle) score(text string, percent float64) string {
	switch {
	case percent >= 90:
		return s.success(text)
	case percent >= 70:
		return s.warn(text)
	default:
		return s.fail(text)
	}
}

func (s ansiStyle) boolValue(v bool) string {
	if v {
		return s.success("yes")
	}
	return s.dim("no")
}
