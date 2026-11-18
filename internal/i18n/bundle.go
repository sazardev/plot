package i18n

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed active.*.toml
var messageFiles embed.FS

// Localizer localizes message IDs at the CLI/TUI edge.
type Localizer struct {
	loc *i18n.Localizer
}

// New loads the embedded message files and resolves tag. Invalid or unknown
// tags fall back to English.
func New(tag string) (*Localizer, error) {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	paths, err := fs.Glob(messageFiles, "active.*.toml")
	if err != nil {
		return nil, fmt.Errorf("list message files: %w", err)
	}
	for _, path := range paths {
		if _, err := bundle.LoadMessageFileFS(messageFiles, path); err != nil {
			return nil, fmt.Errorf("load %s: %w", path, err)
		}
	}

	if tag == "" {
		tag = "en"
	}
	return &Localizer{loc: i18n.NewLocalizer(bundle, tag)}, nil
}

// T returns the localized message. A missing ID returns the ID itself; it
// never panics, per AGENTS.md.
func (l *Localizer) T(id string, data map[string]any) string {
	if l == nil || l.loc == nil {
		return id
	}
	message, err := l.loc.Localize(&i18n.LocalizeConfig{
		MessageID:    id,
		TemplateData: data,
	})
	if err != nil || message == "" {
		return id
	}
	return message
}
