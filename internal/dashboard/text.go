package dashboard

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cosgroveb/guhd/internal/source"
)

// plainText removes terminal instructions before any external text reaches a view.
func plainText(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == '\u202e' || r == '\u202d' || r == '\u202a' || r == '\u202b' || r == '\u202c' || (r >= '\u2066' && r <= '\u2069') {
			return -1
		}
		return r
	}, s)
}
func oneLine(s string) string { return strings.Join(strings.Fields(plainText(s)), " ") }
func age(now, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
func messageText(d source.MessageDetail) string {
	return fmt.Sprintf("From: %s\nTo: %s\nDate: %s\n\n%s", d.Message.From, d.To, d.Message.DateText, d.Body)
}
func (m *model) openDetail() tea.Cmd {
	if m.length(m.section) == 0 {
		return nil
	}
	d := &detailView{id: m.selectedID(m.section), viewport: viewport.New()}
	m.request++
	d.request = m.request
	n := m.selected[m.section]
	switch m.section {
	case 0:
		e := m.events[n]
		d.title = e.Title
		d.target = e.URL
		d.text = fmt.Sprintf("%s – %s\nLocation: %s\nAttendees: %s\nMeeting: %s\n\n%s", e.Start.Format(time.RFC1123), e.End.Format(time.RFC1123), e.Location, strings.Join(e.Attendees, ", "), strings.Join(e.MeetingLinks, "\n"), e.Description)
	case 1:
		r := m.mail[n]
		d.title = r.Subject
		d.target = r.URL
		d.loading = true
		d.text = "Loading message…"
	case 2:
		p := m.projects[n]
		d.title = p.Name
		d.target = p.Path
		d.local = true
		d.text = fmt.Sprintf("%s\n%s\n\n%s\n%s", p.Time.Format(time.RFC1123), p.Revision, p.Body, p.Error)
	}
	m.detail = d
	m.status = ""
	m.resizeDetail()
	if m.section == 1 {
		ctx, c, a, id, req := m.ctx, m.client, m.account(), d.id, d.request
		return func() tea.Msg { detail, err := c.Detail(ctx, a, id); return detailMsg{req, id, detail, err} }
	}
	return nil
}
func (m *model) resizeDetail() {
	if d := m.detail; d != nil {
		d.viewport.SetWidth(max(1, m.width))
		d.viewport.SetHeight(max(1, m.height-5))
		text := plainText(d.text)
		if len(text) > 128*1024 {
			text = text[:128*1024] + "\n[Preview truncated]"
		}
		if d.target != "" {
			text = "Link/path: " + plainText(d.target) + "\n\n" + text
		}
		text = plainText(d.title) + "\n\n" + text
		d.viewport.SetContent(ansi.Hardwrap(text, max(1, m.width), true))
	}
}
func (m *model) target() (string, bool) {
	if m.detail != nil {
		return m.detail.target, m.detail.local
	}
	if m.length(m.section) == 0 {
		return "", false
	}
	n := m.selected[m.section]
	switch m.section {
	case 0:
		return m.events[n].URL, false
	case 1:
		return m.mail[n].URL, false
	default:
		return m.projects[n].Path, true
	}
}
func (m *model) action(key string) tea.Cmd {
	target, local := m.target()
	if target == "" {
		m.status = "No link available"
		return nil
	}
	if key == "y" {
		m.status = "Copy requested: " + plainText(target)
		return tea.SetClipboard(target)
	}
	if !local {
		u, err := url.Parse(target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			m.status = "Only http/https links can be opened"
			return nil
		}
	}
	ctx := m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		opener := "xdg-open"
		if runtime.GOOS == "darwin" {
			opener = "open"
		}
		err := exec.CommandContext(ctx, opener, target).Run()
		if err != nil {
			err = fmt.Errorf("open failed: %w; link: %s", err, target)
		}
		return actionMsg{err}
	}
}
