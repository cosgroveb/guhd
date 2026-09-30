package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) View() tea.View {
	var text string
	switch {
	case m.width < 20 || m.height < 8:
		text = "Enlarge pane to 20×8"
	case m.setup != nil:
		text = m.setup.View(m.width, m.height)
	case m.help:
		text = "guhd help\n\n↑/↓ j/k  select or scroll\nTab      next section\nEnter    details\nEsc      back\nr        refresh\no        open original\ny        request link/path copy\nn        next inbox page\nq Ctrl-C quit\n?        close help"
	case m.detail != nil:
		d := m.detail
		text = oneLine(d.title) + "\n" + d.viewport.View() + "\n" + oneLine(m.status) + "\nEsc back · o open · y copy · ? help"
	default:
		text = m.overview()
	}
	// Apply a final bound even to setup, help, and very long fallback links.
	lines := strings.Split(text, "\n")
	if len(lines) > m.height && m.height > 0 {
		lines = lines[:m.height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(1, m.width), "…")
	}
	v := tea.NewView(lipgloss.NewStyle().Render(strings.Join(lines, "\n")))
	v.AltScreen = true
	return v
}
func (m *model) overview() string {
	lines := []string{m.now.Format("Monday 02 January  15:04"), ""}
	sections := m.sectionCount()
	if m.height < 12 && m.section != 2 {
		sections = 2
	}
	budget := max(sections, m.height-3-sections)
	counts := [3]int{1, 1, 1}
	for extra := budget - sections; extra > 0; extra-- {
		selected := 0
		if counts[0] >= max(1, m.length(0)) || counts[1] < counts[0] {
			selected = 1
		}
		if sections == 3 && counts[0] >= 3 && counts[1] >= 3 && counts[2] < max(1, m.length(2)) {
			selected = 2
		}
		counts[selected]++
	}
	for section := 0; section < sections; section++ {
		title := []string{"UPCOMING", "INBOX", "RECENT PROJECTS"}[section]
		if section == m.section {
			title = "> " + title
		}
		state := m.states[section]
		if state.err != nil {
			label := "failed"
			if !state.success.IsZero() {
				label = "stale · updated " + age(m.now, state.success) + " ago"
			}
			title += " · " + label + ": " + oneLine(state.err.Error())
		} else if state.loading {
			title += " · loading"
		} else if !state.success.IsZero() {
			title += " · updated " + age(m.now, state.success) + " ago"
		}
		if section == 1 {
			title += " · " + m.countLabel()
			if m.nextPage != "" {
				title += " · n more"
			}
		}
		lines = append(lines, title)
		if m.length(section) == 0 {
			label := []string{"No upcoming events", "Inbox empty", "No projects"}[section]
			if state.err != nil {
				label = "Unable to update"
			} else if state.success.IsZero() {
				label = "Loading…"
			}
			lines = append(lines, "  "+label)
		} else {
			n := counts[section]
			start := max(0, m.selected[section]-n+1)
			end := min(m.length(section), start+n)
			for i := start; i < end; i++ {
				prefix := "  "
				if section == m.section && i == m.selected[section] {
					prefix = "› "
				}
				row := m.row(section, i)
				if i == end-1 && end < m.length(section) {
					row = fmt.Sprintf("[%d more] ", m.length(section)-end) + row
				}
				lines = append(lines, prefix+row)
			}
		}
	}
	footer := "? help"
	if m.section == 1 && m.nextPage != "" {
		footer = "n next inbox page · ? help"
	}
	if m.status != "" {
		footer = oneLine(m.status)
	}
	return strings.Join(append(lines, footer), "\n")
}
func (m *model) countLabel() string {
	if m.cfg.MailQuery == "in:inbox" {
		s := m.states[3]
		if s.err != nil {
			return "counts unavailable: " + oneLine(s.err.Error())
		}
		if s.success.IsZero() {
			return "counts loading"
		}
		return fmt.Sprintf("%d unread / %d total", m.counts.Unread, m.counts.Total)
	}
	unread := 0
	for _, r := range m.mail {
		if r.Unread {
			unread++
		}
	}
	suffix := ""
	if m.nextPage != "" {
		suffix = "; more available"
	}
	return fmt.Sprintf("%d unread among %d loaded%s", unread, len(m.mail), suffix)
}
func (m *model) row(section, n int) string {
	switch section {
	case 0:
		e := m.events[n]
		label := e.Start.Local().Format("Mon 02 Jan 15:04")
		if e.Start.Local().Format(time.DateOnly) == m.now.Local().Format(time.DateOnly) {
			label = e.Start.Local().Format("15:04")
		}
		if e.AllDay {
			label = e.Start.Format("Mon 02 Jan") + " all-day"
			if e.Start.Format(time.DateOnly) == m.now.Local().Format(time.DateOnly) {
				label = "Today all-day"
			}
		} else if !e.Start.After(m.now) && e.End.After(m.now) {
			label += " current"
		} else if n == m.nextAppointment() {
			countdown := age(e.Start, m.now)
			if e.Start.Sub(m.now) < time.Minute {
				countdown = "<1m"
			}
			label += " in " + countdown
		}
		if e.Conflict {
			label += " conflict"
		}
		return label + "  " + oneLine(e.Title)
	case 1:
		r := m.mail[n]
		mark := " "
		if r.Unread {
			mark = "●"
		}
		when := age(m.now, r.Received)
		if r.Received.IsZero() {
			when = oneLine(r.DateText)
		}
		return mark + " " + oneLine(r.From) + " · " + oneLine(r.Subject) + "  " + when
	default:
		p := m.projects[n]
		if p.Error != "" {
			return oneLine(p.Name) + " · " + oneLine(p.Error)
		}
		return oneLine(p.Name) + " · " + oneLine(p.Subject) + "  " + age(m.now, p.Time)
	}
}

func (m *model) nextAppointment() int {
	for i, e := range m.events {
		if !e.AllDay && e.Start.After(m.now) {
			return i
		}
	}
	return -1
}
