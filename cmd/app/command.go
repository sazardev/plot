package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sazar/plot/internal/i18n"
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
	cmd.Flags().BoolP("help", "h", false, loc.T("cli.flag.help", nil))
	cmd.Flags().Bool("version", false, loc.T("cli.flag.version", nil))

	cmd.Version = versionText()
	cmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	cmd.SetHelpFunc(func(c *cobra.Command, _ []string) {
		fmt.Fprint(c.OutOrStdout(), helpText(c, loc))
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
