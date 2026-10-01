package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cosgroveb/guhd/internal/theme"
)

// contentSize is shared by text inputs, details and image placement.
func (m *model) contentSize() (int, int) {
	chrome := m.chrome()
	width, height := m.width-2*chrome.Padding, m.height
	if chrome.Border != "" && chrome.Border != "none" {
		width -= 2
		height -= 2
	}
	return max(1, width), max(1, height)
}
func (m *model) chrome() theme.Chrome {
	chrome := m.styles.Chrome
	width, height := m.width-2*chrome.Padding, m.height
	if chrome.Border != "" && chrome.Border != "none" {
		width -= 2
		height -= 2
	}
	minimum := 20
	if m.setup != nil {
		minimum = 24
	}
	if width < minimum || height < 8 {
		return theme.Chrome{}
	}
	return chrome
}
func (m *model) View() tea.View {
	width, height := m.contentSize()
	var text string
	switch {
	case m.width < 20 || m.height < 8:
		text = m.styles.Warning.Render("Enlarge pane to 20×8")
	case m.setup != nil:
		text = m.setup.View(width, height)
	case m.help:
		text = m.styles.Heading.Render("guhd help") + "\n\n" + m.styles.Normal.Render("↑/↓ j/k  select or scroll\nTab      next section\nEnter    details\nEsc      back\nr        refresh\no        open original\ny        request link/path copy\nn        next inbox page\nq Ctrl-C quit\n?        close help")
		if m.preview {
			text += "\n" + m.styles.Muted.Render("s        preview setup (no saves)")
		}
	case m.detail != nil:
		d := m.detail
		text = m.styles.Heading.Render(oneLine(d.title)) + "\n" + d.viewport.View() + "\n" + m.styles.Muted.Render(oneLine(m.status)) + "\n" + m.styles.Footer.Render("Esc back · o open · y copy · ? help")
	default:
		text = m.overview()
	}
	text = m.frame(text, width, height)
	v := tea.NewView(text)
	v.AltScreen = true
	return v
}

// Style only added cells here. A detail viewport may contain trusted image colors.
func (m *model) frame(text string, width, height int) string {
	chrome := m.chrome()
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	_, transparent := m.styles.Normal.GetBackground().(lipgloss.NoColor)
	fill := !transparent || chrome.Padding > 0 || chrome.Separators || (chrome.Border != "" && chrome.Border != "none")
	border := lipgloss.Border{}
	switch chrome.Border {
	case "rounded":
		border = lipgloss.RoundedBorder()
	case "square":
		border = lipgloss.NormalBorder()
	case "double":
		border = lipgloss.DoubleBorder()
	}
	if fill {
		for len(lines) < height {
			lines = append(lines, "")
		}
	}
	for i, line := range lines {
		line = ansi.Truncate(line, width, "…")
		if fill {
			line += m.styles.Normal.Render(strings.Repeat(" ", max(0, width-ansi.StringWidth(line))))
		}
		padding := m.styles.Normal.Render(strings.Repeat(" ", chrome.Padding))
		lines[i] = padding + line + padding
		if border.Left != "" {
			lines[i] = m.styles.Border.Render(border.Left) + lines[i] + m.styles.Border.Render(border.Right)
		}
	}
	if border.Left != "" {
		n := width + 2*chrome.Padding
		top := m.styles.Border.Render(border.TopLeft + strings.Repeat(border.Top, n) + border.TopRight)
		bottom := m.styles.Border.Render(border.BottomLeft + strings.Repeat(border.Bottom, n) + border.BottomRight)
		lines = append([]string{top}, append(lines, bottom)...)
	}
	return strings.Join(lines, "\n")
}
func (m *model) overview() string {
	_, height := m.contentSize()
	chrome := m.chrome()
	lines := []string{m.styles.Muted.Render(m.now.Format("Monday 02 January  15:04")), ""}
	sections := m.sectionCount()
	if height < 12 && m.section != 2 {
		sections = 2
	}
	separators := 0
	if chrome.Separators {
		separators = sections - 1
	}
	budget := max(sections, height-3-sections-separators)
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
		titleStyle := m.styles.Heading
		if state.err != nil {
			titleStyle = m.styles.Error
			if !state.success.IsZero() {
				titleStyle = m.styles.Warning
			}
		}
		if section > 0 && chrome.Separators {
			width, _ := m.contentSize()
			lines = append(lines, m.styles.Border.Render(strings.Repeat("─", width)))
		}
		lines = append(lines, titleStyle.Render(title))
		if m.length(section) == 0 {
			label := []string{"No upcoming events", "Inbox empty", "No projects"}[section]
			if state.err != nil {
				label = "Unable to update"
			} else if state.success.IsZero() {
				label = "Loading…"
			}
			lines = append(lines, m.styles.Muted.Render("  "+label))
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
				style := m.styles.Normal
				switch {
				case section == m.section && i == m.selected[section]:
					style = m.styles.Selected
				case section == 0 && m.events[i].Conflict:
					style = m.styles.Warning
				case section == 0:
					style = m.styles.Upcoming
				case section == 1 && m.mail[i].Unread:
					style = m.styles.Unread
				case section == 2 && m.projects[i].Error != "":
					style = m.styles.Error
				}
				width, _ := m.contentSize()
				lines = append(lines, styledLine(style, prefix+row, width))
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
	if m.preview && m.status == "" {
		footer = "Preview · s setup · ? help"
	}
	width, _ := m.contentSize()
	if _, transparent := m.styles.Normal.GetBackground().(lipgloss.NoColor); !transparent || chrome.Padding > 0 || chrome.Separators || (chrome.Border != "" && chrome.Border != "none") {
		for len(lines) < height-1 {
			lines = append(lines, "")
		}
	}
	return strings.Join(append(lines, styledLine(m.styles.Footer, footer, width)), "\n")
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

func styledLine(style lipgloss.Style, text string, width int) string {
	if _, transparent := style.GetBackground().(lipgloss.NoColor); !transparent {
		text = ansi.Truncate(text, width, "…")
		text += strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
	}
	return style.Render(text)
}
