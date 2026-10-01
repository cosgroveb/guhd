package dashboard

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cosgroveb/guhd/internal/config"
	"github.com/cosgroveb/guhd/internal/source"
	"github.com/cosgroveb/guhd/internal/theme"
)

// NewPreview displays fictional data without configuration or external commands.
func NewPreview(ctx context.Context, styles theme.Styles) tea.Model {
	cfg := config.Defaults()
	cfg.Account = "alex@example.test"
	cfg.Calendars = []string{"preview"}
	cfg.Projects = []string{"/fictional/project"}
	m := New(ctx, cfg, "", false, styles).(*model)
	m.preview = true
	m.client = nil
	m.now = time.Date(2026, time.October, 1, 14, 0, 0, 0, time.Local)
	m.events = []source.Event{
		{ID: "current", Title: "Design review", Start: m.now.Add(-15 * time.Minute), End: m.now.Add(15 * time.Minute), Location: "Studio", Description: "Review the next dashboard layout.", Conflict: true},
		{ID: "next", Title: "Coffee with Sam", Start: m.now.Add(10 * time.Minute), End: m.now.Add(40 * time.Minute), Conflict: true},
		{ID: "later", Title: "An afternoon walk", Start: m.now.Add(time.Hour), End: m.now.Add(2 * time.Hour)},
	}
	m.mail = []source.Message{
		{ID: "preview-image", From: "Morgan", Subject: "A view from the trail", Received: m.now.Add(-12 * time.Minute), Unread: true},
		{ID: "agenda", From: "Sam", Subject: "Agenda for tomorrow", Received: m.now.Add(-time.Hour), Unread: true},
		{ID: "notes", From: "Robin", Subject: "Notes from our last review", Received: m.now.Add(-3 * time.Hour)},
	}
	m.projects = []source.Project{{Name: "example", Path: "/fictional/project", Subject: "Refine the overview", Time: m.now.Add(-2 * time.Hour)}}
	m.counts = source.MailCounts{Total: 3, Unread: 2}
	for i := range m.states {
		m.states[i].success = m.now.Add(-5 * time.Minute)
	}
	m.states[0].err = errors.New("offline (sample)")
	m.states[2].success = time.Time{}
	m.states[2].err = errors.New("directory unavailable (sample)")
	return m
}

func (m *model) previewSetup() {
	s := newSetup(m.ctx, m.cfg, "", nil, m.styles)
	s.preview = true
	s.loading = false
	s.accounts = []source.Account{{Email: "alex@example.test", Client: "default"}}
	s.calendars = []source.Calendar{{ID: "preview", Name: "Personal"}, {ID: "work", Name: "Work"}}
	m.setup = &s
	width, height := m.contentSize()
	updated, _ := s.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m.setup = &updated
}
