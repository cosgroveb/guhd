package dashboard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cosgroveb/guhd/internal/config"
	"github.com/cosgroveb/guhd/internal/theme"
)

func previewModel(t *testing.T, name string) *model {
	t.Helper()
	styles, err := theme.Load(name)
	if err != nil {
		t.Fatal(err)
	}
	m := NewPreview(context.Background(), styles).(*model)
	t.Cleanup(m.cancel)
	return m
}

func assertBounds(t *testing.T, m *model) string {
	t.Helper()
	view := m.View().Content
	if lines := strings.Split(view, "\n"); len(lines) > m.height {
		t.Fatalf("height %d exceeds %d: %q", len(lines), m.height, view)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatalf("line exceeds %d columns: %q", m.width, line)
		}
	}
	return ansi.Strip(view)
}

func TestThemeViewsAndGeometry(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	for _, name := range []string{"plain", "moni-chrome"} {
		for _, size := range [][2]int{{20, 8}, {24, 10}, {71, 32}, {189, 25}, {189, 51}} {
			m := previewModel(t, name)
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			overview := assertBounds(t, m)
			if !strings.Contains(overview, "INBOX") || !strings.Contains(overview, "› ") {
				t.Fatal(overview)
			}
			if name == "moni-chrome" && size[0] >= 71 && !strings.Contains(overview, "╭") {
				t.Fatal("missing frame")
			}
			m.Update(key("?"))
			assertBounds(t, m)
			m.Update(key("?"))
			m.section = 1
			m.Update(key("enter"))
			width, height := m.contentSize()
			if m.detail.viewport.Width() != width || m.detail.viewport.Height() != height-5 {
				t.Fatal("viewport ignores chrome")
			}
			assertBounds(t, m)
			m.detail.viewport.GotoBottom()
			m.Update(tea.WindowSizeMsg{Width: 24, Height: 10})
			assertBounds(t, m)
			if m.detail.viewport.YOffset() < 0 {
				t.Fatal("negative scroll offset")
			}
			m.Update(key("esc"))
			m.Update(key("s"))
			assertBounds(t, m)
		}
	}
}

func TestPlainThemePreservesUnframedView(t *testing.T) {
	m := testModel(t)
	want := m.now.Format("Monday 02 January  15:04") + "\n\nUPCOMING\n  Loading…\n> INBOX · counts loading\n  Loading…\n? help"
	if got := m.View().Content; got != want {
		t.Fatalf("plain changed:\n%q\nwant:\n%q", got, want)
	}
	styles, err := theme.Load("plain")
	if err != nil {
		t.Fatal(err)
	}
	m.styles = styles
	if got := m.View().Content; got != want {
		t.Fatalf("named plain changed: %q", got)
	}
}

func TestThemedSetupSuppressesChromeToKeepControls(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m := previewModel(t, "moni-chrome")
	m.Update(tea.WindowSizeMsg{Width: 24, Height: 10})
	m.Update(key("s"))
	view := assertBounds(t, m)
	if !strings.Contains(view, "Choose an account") || !strings.Contains(view, "> alex@example.test") || strings.Contains(view, "╭") {
		t.Fatal(view)
	}
	m.Update(key("enter"))
	m.Update(key("enter"))
	view = assertBounds(t, m)
	if !strings.Contains(view, "Gmail query") || !strings.Contains(view, "in:inbox") {
		t.Fatal(view)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m.Update(tea.KeyPressMsg{Text: strings.Repeat("x", 100) + "TAIL", Code: 'x'})
	view = assertBounds(t, m)
	if !strings.Contains(view, "TAIL") {
		t.Fatal(view)
	}
}

func TestPreviewNeverPerformsExternalWorkOrSaves(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := previewModel(t, "moni-chrome")
	// A nil client causes an immediate failure if preview accidentally fetches.
	if m.Init() != nil || m.refresh(true) != nil {
		t.Fatal("preview scheduled source refresh")
	}
	m.Update(clockMsg(m.now.Add(time.Hour)))
	for _, k := range []string{"r", "o", "y", "n"} {
		_, cmd := m.Update(key(k))
		if cmd != nil {
			t.Fatalf("%s scheduled external action", k)
		}
	}
	m.section = 1
	if _, cmd := m.Update(key("enter")); cmd != nil {
		t.Fatal("preview requested body")
	}
	if m.detail.loading || !strings.Contains(m.detail.text, "fictional") {
		t.Fatal("preview body unavailable")
	}
	m.Update(key("esc"))
	m.Update(key("s"))
	path := filepath.Join(t.TempDir(), "config.json")
	m.setup.path = path
	for range 2 {
		if _, cmd := m.Update(key("r")); cmd != nil || m.setup.loading {
			t.Fatal("preview retry scheduled discovery or blocked setup")
		}
		m.Update(key("enter"))
	}
	m.Update(key("enter"))
	_, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("setup preview did not finish")
	}
	m.Update(cmd())
	if m.setup != nil {
		t.Fatal("setup remained open")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("preview wrote configuration: %v", err)
	}
}

func TestNoColorRetainsStatusAndSelectionCues(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := previewModel(t, "moni-chrome")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := m.View().Content
	if strings.Contains(view, "\x1b") {
		t.Fatal("NO_COLOR styles escaped")
	}
	for _, cue := range []string{"› ", "●", "stale", "failed", "conflict", "╭"} {
		if !strings.Contains(view, cue) {
			t.Fatalf("missing %s", cue)
		}
	}
}

func TestThemeSanitizesTextButPreservesTrustedViewportStyles(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m := previewModel(t, "moni-chrome")
	m.Update(tea.WindowSizeMsg{Width: 71, Height: 32})
	m.mail[0].Subject = "hello\x1b]52;c;attack\a\x1b[31mworld"
	if got := m.View().Content; strings.Contains(got, "attack") || strings.Contains(got, "\x1b[31m") {
		t.Fatal("untrusted escapes rendered")
	}
	m.section = 1
	m.Update(key("enter"))
	// Native image placeholders use exact RGB IDs. Framing must not override them.
	const trusted = "\x1b[38;2;1;2;3mplaceholder\x1b[0m"
	m.detail.viewport.SetContent(trusted)
	if !strings.Contains(m.View().Content, trusted) {
		t.Fatal("trusted viewport color changed")
	}
}

func TestSetupThemeOverrideDoesNotPersist(t *testing.T) {
	styles, err := theme.Load("moni-chrome")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Account = "alex@example.test"
	path := filepath.Join(t.TempDir(), "config.json")
	m := New(context.Background(), cfg, path, true, styles).(*model)
	t.Cleanup(m.cancel)
	m.setup.step = setupProjects
	_, cmd := m.setup.save()
	if cmd == nil {
		t.Fatal("save failed")
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "plain" {
		t.Fatalf("override persisted: %q", got.Theme)
	}
}
