package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestBuiltins(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	plain, err := Load("")
	if err != nil || plain.Normal.Render("plain") != "plain" || plain.Chrome.Padding != 0 {
		t.Fatalf("plain: %+v, %v", plain, err)
	}
	amber, err := Load("moni-chrome")
	if err != nil {
		t.Fatal(err)
	}
	if !amber.Heading.GetBold() || !amber.Selected.GetBold() || amber.Chrome.Border != "rounded" || !amber.Chrome.Separators {
		t.Fatalf("moni-chrome: %+v", amber)
	}
	if amber.Normal.Render("amber") == "amber" {
		t.Fatal("moni-chrome has no colors")
	}
	t.Setenv("NO_COLOR", "1")
	uncolored, err := Load("moni-chrome")
	if err != nil {
		t.Fatal(err)
	}
	if uncolored.Normal.Render("plain") != "plain" || uncolored.Selected.Render("plain") != "plain" || uncolored.Chrome != amber.Chrome {
		t.Fatal("NO_COLOR must remove colors and attributes while preserving chrome")
	}
}

func TestCustomTheme(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "guhd", "themes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"version":1,"palette":{"ink":"#123456"},"roles":{"normal":{"foreground":"ink","background":"#654321","bold":true},"selected":{"background":"","bold":false}}}`
	if err := os.WriteFile(filepath.Join(dir, "custom.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	styles, err := Load("custom")
	if err != nil {
		t.Fatal(err)
	}
	if styles.Muted.Render("text") != styles.Normal.Render("text") {
		t.Fatal("missing role did not inherit normal")
	}
	if styles.Selected.GetBold() || !styles.Muted.GetBold() {
		t.Fatal("boolean override not preserved")
	}
	if styles.Selected.GetBackground() != lipgloss.NewStyle().GetBackground() {
		t.Fatal("explicit empty background must remain transparent")
	}
	if styles.Selected.GetForeground() != styles.Normal.GetForeground() {
		t.Fatal("foreground not inherited")
	}
	if _, err := Load("missing"); err == nil || !strings.Contains(err.Error(), filepath.Join(dir, "missing.json")) {
		t.Fatalf("missing theme error: %v", err)
	}
	// Reserved built-ins never read user files.
	if err := os.WriteFile(filepath.Join(dir, "plain.json"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("plain"); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidDefinitions(t *testing.T) {
	for _, body := range []string{
		`null`, `{}`, `{"version":2}`, `{"version":1,"extra":true}`,
		`{"version":1} {}`, `{"version":1} trailing`,
		`{"version":1,"roles":{"unknown":{}}}`,
		`{"version":1,"roles":{"normal":{"extra":true}}}`,
		`{"version":1,"roles":{"normal":{"foreground":"red"}}}`,
		`{"version":1,"roles":{"error":{"foreground":"#12345g"}}}`,
		`{"version":1,"palette":{"red":"red"}}`,
		`{"version":1,"palette":{"bad/name":"#123456"}}`,
		`{"version":1,"chrome":{"border":"invisible"}}`,
		`{"version":1,"chrome":{"padding":-1}}`,
		`{"version":1,"chrome":{"padding":3}}`,
		`{"version":1,"chrome":{"padding":1.5}}`,
		`{"version":1,"chrome":{"extra":true}}`,
		strings.Repeat(" ", maxSize+1),
	} {
		t.Run(body[:min(80, len(body))], func(t *testing.T) {
			for _, noColor := range []bool{false, true} {
				if _, err := decode(strings.NewReader(body), noColor); err == nil {
					t.Fatal("invalid definition accepted")
				}
			}
		})
	}
}

func TestNames(t *testing.T) {
	for _, name := range []string{"plain", "moni-chrome", "Custom_2", "-leading"} {
		if !ValidName(name) {
			t.Fatalf("rejected name %q", name)
		}
	}
	for _, name := range []string{"", "..", "../plain", "/plain", "two words", "x.json", "café", "x\x1b"} {
		if ValidName(name) {
			t.Fatalf("accepted name %q", name)
		}
		if name != "" {
			if _, err := Load(name); err == nil {
				t.Fatalf("loaded invalid name %q", name)
			}
		}
	}
}
