package dashboard

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cosgroveb/guhd/internal/config"
	"github.com/cosgroveb/guhd/internal/source"
	"github.com/cosgroveb/guhd/internal/theme"
)

type fixtureSource struct {
	detailCalls, eventsCalls, mailCalls int
	eventsErr                           error
}

func (*fixtureSource) Attachment(context.Context, source.Account, string, source.Attachment) ([]byte, error) {
	return nil, nil
}
func (*fixtureSource) Accounts(context.Context) ([]source.Account, error) { return nil, nil }
func (*fixtureSource) Calendars(context.Context, source.Account) ([]source.Calendar, error) {
	return nil, nil
}
func (f *fixtureSource) Events(context.Context, source.Account, []string, time.Time) ([]source.Event, error) {
	f.eventsCalls++
	return nil, f.eventsErr
}
func (f *fixtureSource) Messages(context.Context, source.Account, string, string) (source.MailPage, error) {
	f.mailCalls++
	return source.MailPage{}, nil
}
func (*fixtureSource) Counts(context.Context, source.Account) (source.MailCounts, error) {
	return source.MailCounts{}, nil
}
func (f *fixtureSource) Detail(_ context.Context, _ source.Account, id string) (source.MessageDetail, error) {
	f.detailCalls++
	return source.MessageDetail{Message: source.Message{ID: id}, Body: "body " + id}, nil
}
func testModel(t *testing.T) *model {
	t.Helper()
	m := New(context.Background(), config.Config{Account: "test@example.com", MailQuery: "in:inbox", RefreshSeconds: 300}, "", false, theme.Styles{}).(*model)
	t.Cleanup(m.cancel)
	m.width, m.height = 80, 24
	m.now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	m.client = &fixtureSource{}
	return m
}
func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}
func TestInitialFocusNavigation(t *testing.T) {
	for _, setup := range []bool{false, true} {
		for _, tc := range []struct {
			name      string
			calendars []string
			section   int
		}{
			{name: "mail only", section: 1},
			{name: "calendar configured", calendars: []string{"primary"}, section: 0},
		} {
			name := tc.name
			if setup {
				name += " after setup"
			}
			t.Run(name, func(t *testing.T) {
				cfg := config.Defaults()
				cfg.Account = "test@example.com"
				cfg.Calendars = tc.calendars
				initial := cfg
				if setup {
					if len(cfg.Calendars) == 0 {
						initial.Calendars = []string{"previous"}
					} else {
						initial.Calendars = nil
					}
				}
				m := New(context.Background(), initial, "", setup, theme.Styles{}).(*model)
				t.Cleanup(m.cancel)
				m.client = &fixtureSource{}
				if setup {
					m.Update(setupSavedMsg{cfg: cfg})
				}
				m.Update(mailMsg{page: source.MailPage{Messages: []source.Message{{ID: "first"}, {ID: "second"}}}})
				if len(cfg.Calendars) > 0 {
					m.Update(eventsMsg{rows: []source.Event{{ID: "first", End: m.now.Add(time.Hour)}, {ID: "second", End: m.now.Add(time.Hour)}}})
				}
				for _, keys := range [][2]string{{"j", "k"}, {"down", "up"}} {
					m.Update(key(keys[0]))
					if m.section != tc.section || m.selected[tc.section] != 1 {
						t.Fatalf("%s: section %d, selection %v", keys[0], m.section, m.selected)
					}
					m.Update(key(keys[1]))
					if m.selected[tc.section] != 0 {
						t.Fatalf("%s: selection %v", keys[1], m.selected)
					}
				}
			})
		}
	}
}
func TestRefreshIsolationAndNoOverlap(t *testing.T) {
	m := testModel(t)
	cmd := m.refresh(false)
	if cmd == nil || !m.states[0].loading || !m.states[1].loading || !m.states[3].loading {
		t.Fatal("initial sources not scheduled")
	}
	if m.refresh(true) != nil {
		t.Fatal("overlapping refresh scheduled")
	}
	m.Update(eventsMsg{rows: []source.Event{{ID: "event", End: m.now.Add(time.Hour)}}})
	if m.states[0].loading || !m.states[1].loading {
		t.Fatal("calendar completion changed mail state")
	}
	m.Update(mailMsg{page: source.MailPage{Messages: []source.Message{{ID: "mail", Subject: "retained"}}}})
	m.Update(countsMsg{err: errors.New("count denied")})
	if len(m.mail) != 1 || !strings.Contains(m.View().Content, "counts unavailable") {
		t.Fatal(m.View().Content)
	}
	m.Update(mailMsg{err: errors.New("offline")})
	view := m.View().Content
	if !strings.Contains(view, "retained") || !strings.Contains(view, "stale") {
		t.Fatal(view)
	}
}
func TestDetailIdentityAndSnapshot(t *testing.T) {
	m := testModel(t)
	m.section = 1
	m.mail = []source.Message{{ID: "a", Subject: "original", URL: "https://example.test/a"}, {ID: "b", Subject: "second"}}
	m.Update(key("down"))
	if m.client.(*fixtureSource).detailCalls != 0 {
		t.Fatal("selection fetched body")
	}
	_, cmd := m.Update(key("enter"))
	first := cmd()
	m.Update(key("esc"))
	m.Update(key("down"))
	_, cmd = m.Update(key("enter"))
	second := cmd()
	m.Update(first)
	if !m.detail.loading {
		t.Fatal("old request applied to reopened item")
	}
	m.Update(second)
	m.detail.viewport.ScrollDown(1)
	offset := m.detail.viewport.YOffset()
	m.Update(mailMsg{page: source.MailPage{Messages: []source.Message{{ID: "x", Subject: "replacement"}}}})
	if m.detail.id != "b" || m.detail.title != "second" || m.detail.viewport.YOffset() != offset || !strings.Contains(m.detail.text, "body b") {
		t.Fatalf("detail changed: %+v", m.detail)
	}
}
func TestSelectionPaginationAndEndBoundary(t *testing.T) {
	m := testModel(t)
	m.section = 1
	m.mail = []source.Message{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	m.selected[1] = 1
	m.Update(mailMsg{page: source.MailPage{Messages: []source.Message{{ID: "c"}, {ID: "a"}, {ID: "b"}}, NextPageToken: "next"}})
	if m.selected[1] != 2 {
		t.Fatal(m.selected)
	}
	m.Update(mailMsg{append: true, page: source.MailPage{Messages: []source.Message{{ID: "d"}}}})
	if m.selected[1] != 2 || len(m.mail) != 4 {
		t.Fatal("pagination lost selection")
	}
	m.Update(mailMsg{page: source.MailPage{Messages: []source.Message{{ID: "x"}}}})
	if m.selected[1] != 0 {
		t.Fatal("selection not clamped")
	}
	m.events = []source.Event{{ID: "ended", End: m.now}, {ID: "current", Start: m.now.Add(-time.Hour), End: m.now.Add(time.Hour), Conflict: true}}
	m.expireEvents()
	if len(m.events) != 1 || !strings.Contains(m.row(0, 0), "current conflict") {
		t.Fatal(m.events)
	}
}
func TestViewsBoundedAndPlain(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := testModel(t)
	m.mail = []source.Message{{ID: "a", Subject: "\x1b[31mDanger\x1b[0m\x1b]52;c;payload\a界界界👩‍💻", From: "Sender\r\nforged"}}
	m.states[1].success = m.now
	for _, size := range [][2]int{{80, 24}, {20, 8}, {40, 12}, {10, 4}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View().Content
		if strings.Contains(view, "\x1b") || strings.Contains(view, "payload") {
			t.Fatal("terminal escape leaked", view)
		}
		lines := strings.Split(view, "\n")
		if len(lines) > size[1] {
			t.Fatalf("height %d: %q", size[1], view)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("width %d: %q", size[0], line)
			}
		}
	}
	if plainText("a\x00\x1b[31mb\n") != "ab\n" {
		t.Fatal("bad plain text")
	}
}
func TestEmptyFailedAndCustomCounts(t *testing.T) {
	m := testModel(t)
	m.Update(mailMsg{})
	if !strings.Contains(m.View().Content, "Inbox empty") {
		t.Fatal(m.View().Content)
	}
	m.states[1] = refreshState{}
	m.Update(mailMsg{err: errors.New("denied")})
	if strings.Contains(m.View().Content, "Inbox empty") {
		t.Fatal("failure displayed as empty")
	}
	m.cfg.MailQuery = "label:work"
	m.mail = []source.Message{{Unread: true}, {Unread: false}}
	m.nextPage = "more"
	if got := m.countLabel(); got != "1 unread among 2 loaded; more available" {
		t.Fatal(got)
	}
}
func TestActionsAndResize(t *testing.T) {
	m := testModel(t)
	m.mail = []source.Message{{ID: "a", Subject: "mail", URL: "javascript:alert(1)"}}
	m.section = 1
	if m.action("o") != nil || !strings.Contains(m.status, "http/https") {
		t.Fatal("unsafe URL allowed")
	}
	m.mail[0].URL = "https://example.test/a"
	if m.action("y") == nil || !strings.Contains(m.status, "Copy requested") {
		t.Fatal("copy request missing")
	}
	m.openDetail()
	m.detail.text = strings.Repeat("long body\n", 100)
	m.resizeDetail()
	m.detail.viewport.ScrollDown(4)
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	if m.detail.viewport.Height() != 5 || m.detail.viewport.YOffset() != 4 {
		t.Fatal("resize lost scroll or dimensions")
	}
	m.Update(key("esc"))
	if m.detail != nil || m.selected[1] != 0 {
		t.Fatal("back lost position")
	}
}

func TestCalendarDayCountdownAndScrollableLink(t *testing.T) {
	m := testModel(t)
	zone := time.FixedZone("east", 14*60*60)
	m.events = []source.Event{{AllDay: true, Start: time.Date(2026, 10, 1, 0, 0, 0, 0, zone), End: m.now.Add(24 * time.Hour)}, {Start: m.now.Add(time.Hour), End: m.now.Add(2 * time.Hour), Title: "next"}}
	if !strings.Contains(m.row(0, 0), "Thu 01 Oct all-day") || !strings.Contains(m.row(0, 1), "in 1h") {
		t.Fatal(m.row(0, 0), m.row(0, 1))
	}
	m.section = 1
	m.mail = []source.Message{{ID: "a", URL: "https://example.test/" + strings.Repeat("long", 100)}}
	m.width = 20
	m.height = 8
	m.openDetail()
	if !strings.Contains(m.detail.viewport.View(), "Link/path:") {
		t.Fatal("link is not in viewport")
	}
	m.detail.viewport.GotoBottom()
	if !strings.Contains(m.detail.viewport.View(), "Loading") {
		t.Fatal("body below long link is inaccessible")
	}
}

func TestFullUnicodeTitleAccessibleInEveryDetail(t *testing.T) {
	for _, section := range []int{0, 1, 2} {
		t.Run([]string{"event", "message", "project"}[section], func(t *testing.T) {
			m := testModel(t)
			m.width = 20
			m.height = 8
			m.section = section
			title := strings.Repeat("会議資料", 12) + "最終文字"
			m.events = []source.Event{{ID: "event", Title: title}}
			m.mail = []source.Message{{ID: "message", Subject: title}}
			m.projects = []source.Project{{Path: "/project", Name: title}}
			cmd := m.openDetail()
			if cmd != nil {
				m.Update(cmd())
			}
			var displayed strings.Builder
			for {
				lines := strings.Split(m.detail.viewport.View(), "\n")
				displayed.WriteString(strings.TrimRight(lines[0], " "))
				before := m.detail.viewport.YOffset()
				m.detail.viewport.ScrollDown(1)
				if m.detail.viewport.YOffset() == before {
					for _, line := range lines[1:] {
						displayed.WriteString(strings.TrimRight(line, " "))
					}
					break
				}
			}
			if !strings.Contains(displayed.String(), title) {
				t.Fatalf("full title inaccessible: %q", displayed.String())
			}
		})
	}
}

func TestFutureEventsHaveDistinctCalendarDates(t *testing.T) {
	m := testModel(t)
	m.events = []source.Event{{Start: m.now.Add(time.Hour)}, {Start: m.now.AddDate(0, 0, 7)}, {Start: m.now.AddDate(0, 0, 14)}, {Start: m.now.AddDate(0, 0, 7), AllDay: true}, {Start: m.now.AddDate(0, 0, 14), AllDay: true}}
	if !strings.HasPrefix(m.row(0, 0), "13:00") {
		t.Fatal(m.row(0, 0))
	}
	for _, n := range []int{1, 3} {
		if !strings.Contains(m.row(0, n), "07 Oct") || !strings.Contains(m.row(0, n+1), "14 Oct") {
			t.Fatal(m.row(0, n), m.row(0, n+1))
		}
	}
}

func TestAppointmentCountdownBoundaries(t *testing.T) {
	for _, tc := range []struct {
		offset time.Duration
		want   string
	}{
		{-time.Second, "current"}, {0, "current"}, {time.Nanosecond, "in <1m"}, {30 * time.Second, "in <1m"}, {time.Minute - time.Nanosecond, "in <1m"}, {time.Minute, "in 1m"},
	} {
		t.Run(tc.offset.String(), func(t *testing.T) {
			m := testModel(t)
			m.events = []source.Event{{Title: "Soon", Start: m.now.Add(tc.offset), End: m.now.Add(time.Hour)}}
			if row := m.row(0, 0); !strings.Contains(row, tc.want+"  Soon") {
				t.Fatalf("row=%q want=%q", row, tc.want)
			}
		})
	}
	m := testModel(t)
	for _, offset := range []time.Duration{-time.Second, 0, 30 * time.Second, time.Minute - time.Nanosecond} {
		if got := age(m.now, m.now.Add(-offset)); got != "now" {
			t.Fatalf("age(%s)=%q", offset, got)
		}
	}
}

func TestPeriodicRefreshDeadlines(t *testing.T) {
	m := testModel(t)
	m.cfg.MailQuery = "label:work"
	f := m.client.(*fixtureSource)
	initial := m.refresh(false)().(tea.BatchMsg)
	for _, cmd := range initial {
		msg := cmd()
		if _, ok := msg.(eventsMsg); ok {
			m.Update(msg)
		}
	}
	start := m.now
	interval := time.Duration(m.cfg.RefreshSeconds) * time.Second
	for _, tc := range []struct {
		elapsed time.Duration
		calls   int
	}{
		{time.Second, 1}, {interval - time.Nanosecond, 1}, {interval, 2},
		{2*interval - time.Nanosecond, 2}, {2 * interval, 3},
	} {
		f.eventsErr = errors.New("offline")
		now := start.Add(tc.elapsed)
		_, cmd := m.Update(clockMsg(now))
		due := tc.elapsed == interval || tc.elapsed == 2*interval
		if m.states[0].loading != due {
			t.Fatalf("elapsed=%s calendar loading=%v", tc.elapsed, m.states[0].loading)
		}
		// Execute the source command, leaving the real Tick command uncalled.
		if due {
			batch := cmd().(tea.BatchMsg)
			if len(batch) != 2 {
				t.Fatalf("commands=%d", len(batch))
			}
			m.Update(batch[0]())
		}
		if !m.now.Equal(now) || f.eventsCalls != tc.calls || f.mailCalls != 1 || !m.states[1].loading {
			t.Fatalf("elapsed=%s clock=%s calendar=%d mail=%d loading=%v", tc.elapsed, m.now, f.eventsCalls, f.mailCalls, m.states[1].loading)
		}
	}
}
