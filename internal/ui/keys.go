package ui

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/sazardev/plot/internal/i18n"
)

// KeyMap holds every TUI binding and its help text.
type KeyMap struct {
	Quit      key.Binding
	NextType  key.Binding
	PrevType  key.Binding
	Help      key.Binding
	CloseHelp key.Binding
	Up        key.Binding
	Down      key.Binding
	Left      key.Binding
	Right     key.Binding
}

// DefaultKeyMap builds the bindings with localized help text.
func DefaultKeyMap(loc *i18n.Localizer) KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", loc.T("tui.key.quit", nil)),
		),
		NextType: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", loc.T("tui.key.next", nil)),
		),
		PrevType: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", loc.T("tui.key.prev", nil)),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", loc.T("tui.key.help", nil)),
		),
		CloseHelp: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", loc.T("tui.key.close_help", nil)),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("up/k", loc.T("tui.key.up", nil)),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("down/j", loc.T("tui.key.down", nil)),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("left/h", loc.T("tui.key.left", nil)),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("right/l", loc.T("tui.key.right", nil)),
		),
	}
}

// ShortHelp is shown in the bottom bar.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.NextType, k.Help, k.Quit}
}

// FullHelp is shown when help is open.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Quit, k.NextType, k.PrevType, k.Help, k.CloseHelp},
		{k.Up, k.Down, k.Left, k.Right},
	}
}
