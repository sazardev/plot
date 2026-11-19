package ui

import "github.com/sazardev/plot/internal/domain"

type chartView struct {
	spec     *domain.RenderSpec
	warnings []domain.Warning
}

type loadedMsg struct {
	dataset *domain.Dataset
	source  string
	specs   map[domain.ChartType]chartView
	order   []domain.ChartType
	initial domain.ChartType
}

type loadErrMsg struct {
	err error
}

type animationTickMsg struct{}

type exportedMsg struct {
	path string
	err  error
}
