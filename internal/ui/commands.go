package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sazardev/plot/internal/domain"
)

const (
	// Build at the TUI's minimum chart body (40x20 window minus chrome and
	// border) so a spec chosen here also fits the smallest supported terminal.
	buildWidth  = 36
	buildHeight = 15
)

var chartOrder = []domain.ChartType{
	domain.ChartLine,
	domain.ChartBar,
	domain.ChartPie,
	domain.ChartDonut,
}

func (m Model) loadCmd() tea.Cmd {
	return func() (msg tea.Msg) {
		// A panic in a loader/builder goroutine must not print a stack over the
		// chart: recover here, log only to the debug file and show the error panel.
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("recovered load panic",
					"panic", fmt.Sprint(recovered),
					"stack", string(debug.Stack()),
				)
				msg = loadErrMsg{err: errors.New(m.opts.Locale.T("tui.error.internal", nil))}
			}
		}()
		if m.opts.Load == nil || m.opts.Build == nil {
			return loadErrMsg{err: errors.New("missing loader")}
		}
		ctx := context.Background()
		ds, err := m.opts.Load(ctx)
		if err != nil {
			return loadErrMsg{err: err}
		}

		views := make(map[domain.ChartType]chartView)
		defaultClone := domain.CloneDataset(ds)
		defaultSpec, err := m.opts.Build(defaultClone, m.opts.InitialType, buildWidth, buildHeight)
		if err != nil {
			return loadErrMsg{err: err}
		}
		views[defaultSpec.ChartType] = chartView{
			spec:     defaultSpec,
			warnings: append([]domain.Warning(nil), defaultClone.Warnings...),
		}

		for _, chartType := range chartOrder {
			if _, exists := views[chartType]; exists {
				continue
			}
			clone := domain.CloneDataset(ds)
			spec, err := m.opts.Build(clone, chartType, buildWidth, buildHeight)
			if err != nil {
				continue
			}
			views[chartType] = chartView{
				spec:     spec,
				warnings: append([]domain.Warning(nil), clone.Warnings...),
			}
		}

		order := make([]domain.ChartType, 0, len(views))
		for _, chartType := range chartOrder {
			if _, exists := views[chartType]; exists {
				order = append(order, chartType)
			}
		}
		if len(order) == 0 {
			order = append(order, defaultSpec.ChartType)
		}

		return loadedMsg{
			dataset: ds,
			source:  sourceLabel(ds),
			specs:   views,
			order:   order,
			initial: defaultSpec.ChartType,
		}
	}
}

func (m Model) exportCmd(format string) tea.Cmd {
	return func() (msg tea.Msg) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("recovered export panic",
					"panic", fmt.Sprint(recovered),
					"stack", string(debug.Stack()),
				)
				msg = exportedMsg{err: errors.New(m.opts.Locale.T("tui.export.unavailable", nil))}
			}
		}()
		if m.opts.Export == nil || m.opts.Build == nil || m.dataset == nil || len(m.order) == 0 {
			return exportedMsg{err: errors.New(m.opts.Locale.T("tui.export.unavailable", nil))}
		}
		chartType := m.order[m.current]
		ds := domain.CloneDataset(m.dataset)
		spec, err := m.opts.Build(ds, chartType, 80, 24)
		if err != nil {
			return exportedMsg{err: err}
		}
		path, err := m.opts.Export(spec, format)
		return exportedMsg{path: path, err: err}
	}
}

func sourceLabel(ds *domain.Dataset) string {
	if ds == nil {
		return ""
	}
	return domain.SanitizeText(sourceLabelRaw(ds))
}

func sourceLabelRaw(ds *domain.Dataset) string {
	switch ds.SourceKind {
	case domain.SourceFile:
		base := filepath.Base(ds.SourceID)
		if base == "" || base == "." {
			base = "file"
		}
		if ds.SourceCount > 1 {
			return fmt.Sprintf("%s (+%d more)", base, ds.SourceCount-1)
		}
		return base
	case domain.SourceDB:
		base := filepath.Base(ds.SourceID)
		if base == "" || base == "." {
			return "database"
		}
		return base
	default:
		return "stdin"
	}
}
