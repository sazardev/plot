package ui

import "github.com/sazar/plot/internal/domain"

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
