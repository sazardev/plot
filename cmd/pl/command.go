package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/sazardev/plot/internal/i18n"
)

type cliOptions struct {
	Type       string
	Label      string
	LabelSet   bool
	Columns    []string
	ColumnsSet bool
	Renames    []string
	Sorts      []string
	Filters    []string
	Delimiter  string
	Header     bool
	NoHeader   bool
	Width      int
	Height     int
	Color      string
	ASCII      bool
	Title      string
	TUI        bool
	NoMotion   bool
	Config     string
	DB         string
	Query      string
	QueryFile  string
	Timeout    time.Duration
	CacheDir   string
	NoCache    bool
	CacheTTL   time.Duration
	CacheMax   int64
	Export     string
	Bins       int
	Grid       bool
	NoLegend   bool
	NoTitle    bool
	NoMarkers  bool
	changed    map[string]bool
}

func newCommand(opts *cliOptions, stream appIO, loc *i18n.Localizer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "pl [flags] [file]",
		Short:         loc.T("cli.help.short", nil),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.LabelSet = cmd.Flags().Changed("label")
			opts.ColumnsSet = cmd.Flags().Changed("columns")
			opts.changed = make(map[string]bool)
			cmd.Flags().Visit(func(flag *pflag.Flag) {
				opts.changed[flag.Name] = true
			})
			return runApp(args, opts, stream, loc)
		},
	}
	cmd.SetOut(stream.Out)
	cmd.SetErr(stream.Err)

	cmd.Flags().StringVar(&opts.Type, "type", "", loc.T("cli.flag.type", nil))
	cmd.Flags().StringVar(&opts.Label, "label", "", loc.T("cli.flag.label", nil))
	cmd.Flags().StringSliceVar(&opts.Columns, "columns", nil, loc.T("cli.flag.columns", nil))
	cmd.Flags().StringArrayVar(&opts.Renames, "rename", nil, loc.T("cli.flag.rename", nil))
	cmd.Flags().StringSliceVar(&opts.Sorts, "sort", nil, loc.T("cli.flag.sort", nil))
	cmd.Flags().StringArrayVar(&opts.Filters, "filter", nil, loc.T("cli.flag.filter", nil))
	cmd.Flags().StringVar(&opts.Delimiter, "delimiter", "auto", loc.T("cli.flag.delimiter", nil))
	cmd.Flags().BoolVar(&opts.Header, "header", false, loc.T("cli.flag.header", nil))
	cmd.Flags().BoolVar(&opts.NoHeader, "no-header", false, loc.T("cli.flag.no_header", nil))
	cmd.Flags().IntVar(&opts.Width, "width", 0, loc.T("cli.flag.width", nil))
	cmd.Flags().IntVar(&opts.Height, "height", 0, loc.T("cli.flag.height", nil))
	cmd.Flags().StringVar(&opts.Color, "color", "auto", loc.T("cli.flag.color", nil))
	cmd.Flags().BoolVar(&opts.ASCII, "ascii", false, loc.T("cli.flag.ascii", nil))
	cmd.Flags().StringVar(&opts.Title, "title", "", loc.T("cli.flag.title", nil))
	cmd.Flags().StringVar(&opts.Config, "config", "", loc.T("cli.flag.config", nil))
	cmd.Flags().BoolVar(&opts.TUI, "tui", false, loc.T("cli.flag.tui", nil))
	cmd.Flags().BoolVar(&opts.NoMotion, "no-motion", false, loc.T("cli.flag.no_motion", nil))
	cmd.Flags().BoolVar(&opts.NoCache, "no-cache", false, loc.T("cli.flag.no_cache", nil))
	cmd.Flags().StringVar(&opts.CacheDir, "cache-dir", "", loc.T("cli.flag.cache_dir", nil))
	cmd.Flags().StringVar(&opts.Export, "export", "", loc.T("cli.flag.export", nil))
	cmd.Flags().IntVar(&opts.Bins, "bins", 10, loc.T("cli.flag.bins", nil))
	cmd.Flags().BoolVar(&opts.Grid, "grid", false, loc.T("cli.flag.grid", nil))
	cmd.Flags().BoolVar(&opts.NoLegend, "no-legend", false, loc.T("cli.flag.no_legend", nil))
	cmd.Flags().BoolVar(&opts.NoTitle, "no-title", false, loc.T("cli.flag.no_title", nil))
	cmd.Flags().BoolVar(&opts.NoMarkers, "no-markers", false, loc.T("cli.flag.no_markers", nil))
	cmd.Flags().StringVar(&opts.DB, "db", "", loc.T("cli.flag.db", nil))
	cmd.Flags().StringVar(&opts.Query, "query", "", loc.T("cli.flag.query", nil))
	cmd.Flags().StringVar(&opts.QueryFile, "query-file", "", loc.T("cli.flag.query_file", nil))
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 10*time.Second, loc.T("cli.flag.timeout", nil))
	cmd.Flags().BoolP("help", "h", false, loc.T("cli.flag.help", nil))
	cmd.Flags().Bool("version", false, loc.T("cli.flag.version", nil))

	cmd.Version = versionText()
	cmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	cmd.SetHelpFunc(func(c *cobra.Command, _ []string) {
		_, _ = fmt.Fprint(c.OutOrStdout(), helpText(c, loc))
	})
	return cmd
}

func helpText(cmd *cobra.Command, loc *i18n.Localizer) string {
	var b strings.Builder
	b.WriteString(loc.T("cli.help.preamble", nil))
	b.WriteString("\n\n")

	b.WriteString(loc.T("cli.help.usage_label", nil))
	b.WriteString("\n  ")
	b.WriteString(cmd.UseLine())
	b.WriteString("\n\n")

	b.WriteString(loc.T("cli.help.input_label", nil))
	b.WriteString("\n")
	b.WriteString(loc.T("cli.help.input", nil))
	b.WriteString("\n\n")

	b.WriteString(loc.T("cli.help.types_label", nil))
	b.WriteString("\n")
	b.WriteString(loc.T("cli.help.types", nil))
	b.WriteString("\n\n")

	b.WriteString(loc.T("cli.help.flags_label", nil))
	b.WriteString("\n")
	b.WriteString(strings.TrimRight(cmd.LocalFlags().FlagUsages(), "\n"))
	b.WriteString("\n\n")

	b.WriteString(loc.T("cli.help.examples_label", nil))
	b.WriteString("\n")
	b.WriteString(loc.T("cli.help.examples", nil))
	b.WriteString("\n\n")

	b.WriteString(loc.T("cli.help.exit_label", nil))
	b.WriteString("\n")
	b.WriteString(loc.T("cli.help.exit_codes", nil))
	b.WriteString("\n")
	return b.String()
}
