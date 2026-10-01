// Package theme loads declarative terminal styles.
package theme

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"charm.land/lipgloss/v2"
)

//go:embed builtins/*.json
var builtins embed.FS

const maxSize = 64 * 1024

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Chrome struct {
	Border     string `json:"border"`
	Padding    int    `json:"padding"`
	Separators bool   `json:"separators"`
}

type Styles struct {
	Normal, Muted, Heading, Selected, Unread, Upcoming, Warning, Error, Border, Footer lipgloss.Style
	Chrome                                                                             Chrome
}

type role struct {
	Foreground *string `json:"foreground"`
	Background *string `json:"background"`
	Bold       *bool   `json:"bold"`
	Italic     *bool   `json:"italic"`
	Underline  *bool   `json:"underline"`
}

type definition struct {
	Version int               `json:"version"`
	Palette map[string]string `json:"palette"`
	Roles   map[string]role   `json:"roles"`
	Chrome  Chrome            `json:"chrome"`
}

func ValidName(name string) bool { return namePattern.MatchString(name) }

func Load(name string) (Styles, error) {
	if name == "" {
		name = "plain"
	}
	if !ValidName(name) {
		return Styles{}, fmt.Errorf("invalid theme name %q: use letters, digits, hyphens or underscores", name)
	}
	var reader io.Reader
	if name == "plain" || name == "moni-chrome" {
		data, err := builtins.ReadFile("builtins/" + name + ".json")
		if err != nil {
			return Styles{}, fmt.Errorf("read theme %q: %w", name, err)
		}
		reader = bytes.NewReader(data)
	} else {
		dir, err := os.UserConfigDir()
		if err != nil {
			return Styles{}, fmt.Errorf("find theme directory: %w", err)
		}
		path := filepath.Join(dir, "guhd", "themes", name+".json")
		file, err := os.Open(path)
		if err != nil {
			return Styles{}, fmt.Errorf("read theme %q at %s: %w", name, path, err)
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	styles, err := decode(reader, os.Getenv("NO_COLOR") != "")
	if err != nil {
		return Styles{}, fmt.Errorf("load theme %q: %w", name, err)
	}
	return styles, nil
}

func decode(reader io.Reader, noColor bool) (Styles, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxSize+1))
	if err != nil {
		return Styles{}, fmt.Errorf("read theme: %w", err)
	}
	if len(data) > maxSize {
		return Styles{}, errors.New("theme exceeds 64 KiB")
	}
	var def definition
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&def); err != nil {
		return Styles{}, err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected trailing JSON")
		}
		return Styles{}, err
	}
	if def.Version != 1 {
		return Styles{}, errors.New("theme version must be 1")
	}
	switch def.Chrome.Border {
	case "", "none", "rounded", "square", "double":
	default:
		return Styles{}, fmt.Errorf("unknown border %q", def.Chrome.Border)
	}
	if def.Chrome.Padding < 0 || def.Chrome.Padding > 2 {
		return Styles{}, errors.New("chrome padding must be between 0 and 2")
	}
	for name, color := range def.Palette {
		if !ValidName(name) || !colorPattern.MatchString(color) {
			return Styles{}, fmt.Errorf("invalid palette entry %q: expected a name and #RRGGBB color", name)
		}
	}
	var styles Styles
	targets := map[string]*lipgloss.Style{
		"normal": &styles.Normal, "muted": &styles.Muted, "heading": &styles.Heading,
		"selected": &styles.Selected, "unread": &styles.Unread, "upcoming": &styles.Upcoming,
		"warning": &styles.Warning, "error": &styles.Error, "border": &styles.Border, "footer": &styles.Footer,
	}
	for name := range def.Roles {
		if _, ok := targets[name]; !ok {
			return Styles{}, fmt.Errorf("unknown role %q", name)
		}
	}
	normal, err := resolve(def.Roles["normal"], lipgloss.NewStyle(), def.Palette)
	if err != nil {
		return Styles{}, fmt.Errorf("normal role: %w", err)
	}
	for name, target := range targets {
		resolved, err := resolve(def.Roles[name], normal, def.Palette)
		if err != nil {
			return Styles{}, fmt.Errorf("%s role: %w", name, err)
		}
		if !noColor {
			*target = resolved
		}
	}
	styles.Chrome = def.Chrome
	return styles, nil
}

func resolve(r role, base lipgloss.Style, palette map[string]string) (lipgloss.Style, error) {
	for _, field := range []struct {
		value      *string
		background bool
	}{{r.Foreground, false}, {r.Background, true}} {
		if field.value == nil {
			continue
		}
		value := *field.value
		if value == "" {
			if field.background {
				base = base.UnsetBackground()
			} else {
				base = base.UnsetForeground()
			}
			continue
		}
		if !colorPattern.MatchString(value) {
			var ok bool
			value, ok = palette[value]
			if !ok {
				return lipgloss.Style{}, fmt.Errorf("unknown color %q", *field.value)
			}
		}
		if field.background {
			base = base.Background(lipgloss.Color(value))
		} else {
			base = base.Foreground(lipgloss.Color(value))
		}
	}
	if r.Bold != nil {
		base = base.Bold(*r.Bold)
	}
	if r.Italic != nil {
		base = base.Italic(*r.Italic)
	}
	if r.Underline != nil {
		base = base.Underline(*r.Underline)
	}
	return base, nil
}
