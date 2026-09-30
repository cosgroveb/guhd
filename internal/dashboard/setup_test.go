package dashboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cosgroveb/guhd/internal/config"
	"github.com/cosgroveb/guhd/internal/source"
)

type setupSource struct {
	dataSource
	accounts  []source.Account
	calendars []source.Calendar
	err       error
}

func (s setupSource) Accounts(context.Context) ([]source.Account, error) {
	return s.accounts, s.err
}

func (s setupSource) Calendars(context.Context, source.Account) ([]source.Calendar, error) {
	return s.calendars, s.err
}

func setupKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestSetupSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	client := setupSource{
		accounts:  []source.Account{{Email: "alex@example.com", Client: "work"}},
		calendars: []source.Calendar{{ID: "opaque-ID", Name: "Work"}, {ID: "other-ID", Name: "Other"}},
	}
	m := newSetup(context.Background(), config.Defaults(), path, client)
	m, _ = m.Update(m.Init()())
	m, cmd := m.Update(setupKey(tea.KeyEnter))
	if !m.loading || cmd == nil {
		t.Fatal("calendar discovery did not start")
	}
	m, _ = m.Update(cmd())
	m, _ = m.Update(setupKey(tea.KeySpace))
	m, _ = m.Update(setupKey(tea.KeyEnter))
	if m.step != setupQuery || !m.query.Focused() {
		t.Fatal("query input not focused")
	}
	m.query.SetValue("is:unread")
	m, _ = m.Update(setupKey(tea.KeyEnter))
	if !m.projects.Focused() {
		t.Fatal("project input not focused")
	}
	m.projects.SetValue(".; " + t.TempDir())
	m, cmd = m.Update(setupKey(tea.KeyEnter))
	if m.err != nil {
		t.Fatal(m.err)
	}
	if cmd == nil {
		t.Fatal("missing saved message")
	}
	saved, ok := cmd().(setupSavedMsg)
	if !ok {
		t.Fatal("wrong completion message")
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Account != "alex@example.com" || loaded.Client != "work" || loaded.MailQuery != "is:unread" {
		t.Fatalf("unexpected config: %+v", loaded)
	}
	if len(loaded.Calendars) != 1 || loaded.Calendars[0] != "opaque-ID" {
		t.Fatalf("calendars: %v", loaded.Calendars)
	}
	if len(loaded.Projects) != 2 || !filepath.IsAbs(loaded.Projects[0]) {
		t.Fatalf("projects: %v", loaded.Projects)
	}
	if saved.cfg.Account != loaded.Account {
		t.Fatal("saved message differs from disk")
	}
}

func TestSetupCancelPreservesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte("existing configuration")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	m := newSetup(context.Background(), config.Defaults(), path, setupSource{})
	_, cmd := m.Update(setupKey(tea.KeyEscape))
	if _, ok := cmd().(setupCanceledMsg); !ok {
		t.Fatal("missing cancel message")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatal("cancel overwrote config")
	}
}

func TestSetupDiscoveryRetryAndBack(t *testing.T) {
	client := setupSource{err: errors.New("discovery failed")}
	m := newSetup(context.Background(), config.Defaults(), "", client)
	m, _ = m.Update(m.Init()())
	if m.err == nil || !strings.Contains(m.View(80, 20), "discovery failed") {
		t.Fatal("discovery error hidden")
	}
	m.client = setupSource{accounts: []source.Account{{Email: "alex@example.com", Client: "default"}}}
	m, cmd := m.Update(setupKey('r'))
	if !m.loading || cmd == nil {
		t.Fatal("retry did not start")
	}
	m, _ = m.Update(cmd())
	m, cmd = m.Update(setupKey(tea.KeyEnter))
	stale := cmd()
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.step != setupAccount {
		t.Fatal("back did not return to accounts")
	}
	m, _ = m.Update(stale)
	if m.step != setupAccount || len(m.calendars) != 0 {
		t.Fatal("stale calendar response applied")
	}
	m, _ = m.Update(setupKey(tea.KeyEnter))
	m, _ = m.Update(setupCalendarsMsg{request: m.request, err: errors.New("calendar denied")})
	if m.err == nil {
		t.Fatal("calendar failure hidden")
	}
	m, cmd = m.Update(setupKey('r'))
	m, _ = m.Update(cmd())
	if m.err != nil || m.loading {
		t.Fatal("calendar retry failed")
	}
	m, _ = m.Update(setupKey(tea.KeyEnter))
	if m.step != setupQuery {
		t.Fatal("zero calendars must allow mail-only setup")
	}
}

func TestSetupValidationAndSaveError(t *testing.T) {
	m := newSetup(context.Background(), config.Defaults(), t.TempDir(), setupSource{})
	m.step = setupQuery
	m.query.SetValue("  ")
	m, cmd := m.Update(setupKey(tea.KeyEnter))
	if m.err == nil || cmd != nil || m.step != setupQuery {
		t.Fatal("empty query advanced")
	}
	m.query.SetValue("in:inbox")
	m, _ = m.Update(setupKey(tea.KeyEnter))
	m.cfg.Account = "alex@example.com"
	m, cmd = m.Update(setupKey(tea.KeyEnter))
	if m.err == nil || cmd != nil {
		t.Fatal("save to directory did not report error")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.step != setupQuery || m.err != nil || !m.query.Focused() {
		t.Fatal("cannot edit after save failure")
	}
}

func TestSetupKeepsExistingCalendarSelection(t *testing.T) {
	cfg := config.Defaults()
	cfg.Account, cfg.Client, cfg.Calendars = "alex@example.com", "work", []string{"private"}
	client := setupSource{accounts: []source.Account{{Email: cfg.Account, Client: cfg.Client}}, calendars: []source.Calendar{{ID: "private"}}}
	m := newSetup(context.Background(), cfg, "", client)
	m, _ = m.Update(m.Init()())
	m, cmd := m.Update(setupKey(tea.KeyEnter))
	m, _ = m.Update(cmd())
	if !m.selected["private"] {
		t.Fatal("existing calendar selection lost")
	}
}

func TestSetupUnavailableAccountDoesNotBlockHealthyAccount(t *testing.T) {
	client := setupSource{accounts: []source.Account{
		{Email: "old@example.com", Client: "default", Error: "token unavailable"},
		{Email: "alex@example.com", Client: "default"},
	}}
	m := newSetup(context.Background(), config.Defaults(), "", client)
	m, _ = m.Update(m.Init()())
	m, _ = m.Update(setupKey(tea.KeyEnter))
	if m.err == nil {
		t.Fatal("unavailable account did not report error")
	}
	m, _ = m.Update(setupKey(tea.KeyDown))
	m, cmd := m.Update(setupKey(tea.KeyEnter))
	if m.step != setupCalendar || cmd == nil || m.cfg.Account != "alex@example.com" {
		t.Fatal("unavailable account blocked healthy account selection")
	}
}

func TestSetupInputScrollAndVirtualCursor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	for _, step := range []setupStep{setupQuery, setupProjects} {
		m := testModel(t)
		s := newSetup(m.ctx, config.Defaults(), "", m.client)
		s.step = step
		s.loading = false
		s.query.SetValue("")
		if step == setupQuery {
			s.query.Focus()
		} else {
			s.projects.Focus()
		}
		m.setup = &s
		m.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
		value := "HEAD" + strings.Repeat("a", 80) + "TAIL"
		m.Update(tea.KeyPressMsg{Code: 'H', Text: value})
		if !strings.Contains(m.View().Content, "TAIL") {
			t.Fatalf("cursor-end text is hidden: %q", m.View().Content)
		}
		end := virtualCursor(m.View().Content)
		if end == nil {
			t.Fatal("focused input has no visible virtual cursor")
		}
		row := 3
		if step == setupProjects {
			row = 4
		}
		if end.Y != row {
			t.Fatalf("cursor row %d, want %d", end.Y, row)
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		left := virtualCursor(m.View().Content)
		if left == nil || left.X != end.X-1 {
			t.Fatal("left did not move insertion cursor")
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
		if !strings.Contains(ansi.Strip(m.View().Content), "HEAD") || virtualCursor(m.View().Content).X != 2 {
			t.Fatal("home did not show start")
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
		m.Update(tea.WindowSizeMsg{Width: 24, Height: 8})
		if !strings.Contains(m.View().Content, "TAIL") || virtualCursor(m.View().Content).X >= 24 {
			t.Fatal("resize hid active end")
		}
	}
}

func TestSetupUnicodeCursorAndSanitization(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := testModel(t)
	s := newSetup(m.ctx, config.Defaults(), "", m.client)
	s.step = setupQuery
	s.query.SetValue("界界ab")
	s.query.Focus()
	s.query.CursorStart()
	m.setup = &s
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	for _, want := range []int{4, 6, 7} {
		m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		if got := virtualCursor(m.View().Content); got == nil || got.X != want {
			t.Fatalf("cursor %+v, want x=%d", got, want)
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if virtualCursor(m.View().Content).X != 6 {
		t.Fatal("Unicode left cursor misaligned")
	}
	m.setup.query.SetValue(strings.Repeat("界", 40) + "TAIL")
	m.setup.query.CursorEnd()
	m.Update(tea.WindowSizeMsg{Width: 24, Height: 8})
	if !strings.Contains(m.View().Content, "TAIL") || virtualCursor(m.View().Content).X >= 24 {
		t.Fatal("Unicode overflow hid cursor")
	}
	if strings.Contains(m.View().Content, "\x1b[38;") || strings.Contains(m.View().Content, "\x1b[48;") {
		t.Fatal("NO_COLOR or plain text violated")
	}
	m.Update(tea.WindowSizeMsg{Width: 10, Height: 4})
	if virtualCursor(m.View().Content) != nil {
		t.Fatal("tiny hint must not show editing cursor")
	}
}

func TestResizeKeepsMiddleCursorVisible(t *testing.T) {
	m := testModel(t)
	s := newSetup(m.ctx, config.Defaults(), "", m.client)
	s.step = setupQuery
	s.query.SetValue(strings.Repeat("a", 80))
	s.query.CursorEnd()
	s.query.Focus()
	m.setup = &s
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	for range 5 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	m.Update(tea.WindowSizeMsg{Width: 24, Height: 8})
	if cursor := virtualCursor(m.View().Content); cursor == nil || cursor.X >= 24 {
		t.Fatalf("resized cursor outside input: %+v", cursor)
	}
}

func virtualCursor(view string) *tea.Cursor {
	for row, line := range strings.Split(view, "\n") {
		if col := strings.Index(line, "\x1b[7m"); col >= 0 {
			return tea.NewCursor(ansi.StringWidth(line[:col]), row)
		}
	}
	return nil
}

func TestSetupInputSanitizesUntrustedTextBeforeStyling(t *testing.T) {
	m := testModel(t)
	cfg := config.Defaults()
	cfg.MailQuery = "hello\x1b[31mred\x1b[0m\x1b]52;c;payload\a"
	s := newSetup(m.ctx, cfg, "", m.client)
	s.step = setupQuery
	s.query.Focus()
	m.setup = &s
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m.Update(tea.PasteMsg{Content: "\x1b[31mpasted\x1b[0m\u202e\x1b]52;c;attack\a"})
	view := m.View().Content
	if strings.Contains(view, "payload") || strings.Contains(view, "attack") || strings.Contains(view, "\x1b[31m") || strings.Contains(view, "\u202e") {
		t.Fatalf("untrusted styling leaked: %q", view)
	}
	if virtualCursor(view) == nil || !strings.Contains(ansi.Strip(view), "helloredpasted") {
		t.Fatalf("sanitization removed cursor or text: %q", view)
	}
}
