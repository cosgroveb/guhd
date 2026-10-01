package dashboard

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cosgroveb/guhd/internal/config"
	"github.com/cosgroveb/guhd/internal/source"
	"github.com/cosgroveb/guhd/internal/theme"
)

type setupSavedMsg struct{ cfg config.Config }
type setupCanceledMsg struct{}
type setupAccountsMsg struct {
	accounts []source.Account
	err      error
}
type setupCalendarsMsg struct {
	request   int
	calendars []source.Calendar
	err       error
}

type setupStep int

const (
	setupAccount setupStep = iota
	setupCalendar
	setupQuery
	setupProjects
)

type setupModel struct {
	cfg       config.Config
	path      string
	client    dataSource
	ctx       context.Context
	step      setupStep
	accounts  []source.Account
	calendars []source.Calendar
	selected  map[string]bool
	cursor    int
	request   int
	loading   bool
	err       error
	query     textinput.Model
	projects  textinput.Model
	styles    theme.Styles
	preview   bool
}

func newSetup(ctx context.Context, cfg config.Config, path string, client dataSource, styles ...theme.Styles) setupModel {
	var style theme.Styles
	if len(styles) > 0 {
		style = styles[0]
	}
	inputStyle := textinput.StyleState{Text: style.Normal, Placeholder: style.Muted, Suggestion: style.Muted, Prompt: style.Heading}
	inputStyles := textinput.Styles{Focused: inputStyle, Blurred: inputStyle}
	query := textinput.New()
	query.Prompt = "> "
	query.SetStyles(inputStyles)
	query.CharLimit = 4096
	query.SetValue(plainText(cfg.MailQuery))
	projects := textinput.New()
	projects.Prompt = "> "
	projects.SetStyles(inputStyles)
	projects.CharLimit = 16384
	projects.SetValue(plainText(strings.Join(cfg.Projects, "; ")))
	selected := make(map[string]bool)
	for _, id := range cfg.Calendars {
		selected[id] = true
	}
	return setupModel{cfg: cfg, path: path, client: client, ctx: ctx, selected: selected, query: query, projects: projects, loading: true, styles: style}
}

func (m setupModel) Init() tea.Cmd {
	if m.preview {
		return nil
	}
	return func() tea.Msg {
		accounts, err := m.client.Accounts(m.ctx)
		return setupAccountsMsg{accounts: accounts, err: err}
	}
}

func (m *setupModel) discoverCalendars() tea.Cmd {
	if m.preview {
		m.loading = false
		return nil
	}
	m.request++
	request := m.request
	account := source.Account{Email: m.cfg.Account, Client: m.cfg.Client}
	m.loading = true
	m.err = nil
	client, ctx := m.client, m.ctx
	return func() tea.Msg {
		calendars, err := client.Calendars(ctx, account)
		return setupCalendarsMsg{request: request, calendars: calendars, err: err}
	}
}

func (m setupModel) Update(msg tea.Msg) (setupModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		for _, input := range []*textinput.Model{&m.query, &m.projects} {
			position := input.Position()
			input.SetWidth(max(1, msg.Width-4))
			// Rebuild both overflow boundaries when shrinking around a middle cursor.
			input.CursorEnd()
			input.CursorStart()
			input.SetCursor(position)
		}
		return m, nil
	case setupAccountsMsg:
		m.loading, m.err = false, msg.err
		m.accounts = msg.accounts
		if m.err == nil && len(m.accounts) == 0 {
			m.err = fmt.Errorf("no accounts found: run gog auth add EMAIL first")
		}
		for i, account := range m.accounts {
			if account.Email == m.cfg.Account && account.Client == m.cfg.Client {
				m.cursor = i
			}
		}
		return m, nil
	case setupCalendarsMsg:
		if m.step != setupCalendar || msg.request != m.request {
			return m, nil
		}
		m.loading, m.err = false, msg.err
		m.calendars = msg.calendars
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return setupCanceledMsg{} }
		case "shift+tab":
			if m.step > setupAccount {
				m.step--
				m.cursor, m.err, m.loading = 0, nil, false
				m.request++
				m.query.Blur()
				m.projects.Blur()
				if m.step == setupQuery {
					cmd := m.query.Focus()
					return m, cmd
				}
			}
			return m, nil
		}
		if m.step == setupAccount || m.step == setupCalendar {
			return m.updateSelection(msg.String())
		}
		if msg.String() == "enter" {
			m.err = nil
			if m.step == setupQuery {
				if strings.TrimSpace(m.query.Value()) == "" {
					m.err = fmt.Errorf("mail query must not be empty")
					return m, nil
				}
				m.step = setupProjects
				m.query.Blur()
				cmd := m.projects.Focus()
				return m, cmd
			}
			return m.save()
		}
	}
	var cmd tea.Cmd
	switch m.step {
	case setupAccount, setupCalendar:
		return m, nil
	case setupQuery:
		m.query, cmd = updateInput(m.query, msg)
	case setupProjects:
		m.projects, cmd = updateInput(m.projects, msg)
	}
	return m, cmd
}

func (m setupModel) updateSelection(key string) (setupModel, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	if key == "r" {
		if m.preview {
			return m, nil
		}
		m.err, m.cursor = nil, 0
		if m.step == setupAccount {
			m.loading = true
			return m, m.Init()
		}
		cmd := m.discoverCalendars()
		return m, cmd
	}
	count := len(m.accounts)
	if m.step == setupCalendar {
		count = len(m.calendars)
	}
	switch key {
	case "down", "j":
		if m.step == setupAccount && count > 0 && m.accounts[m.cursor].Error != "" {
			m.err = nil
		}
		m.cursor = min(m.cursor+1, max(0, count-1))
	case "up", "k":
		if m.step == setupAccount && count > 0 && m.accounts[m.cursor].Error != "" {
			m.err = nil
		}
		m.cursor = max(0, m.cursor-1)
	case "space":
		if m.step == setupCalendar && count > 0 {
			id := m.calendars[m.cursor].ID
			m.selected[id] = !m.selected[id]
		}
	case "enter":
		if m.err != nil {
			return m, nil
		}
		if m.step == setupAccount {
			if count == 0 {
				return m, nil
			}
			account := m.accounts[m.cursor]
			if account.Error != "" {
				m.err = fmt.Errorf("account unavailable: %s", account.Error)
				return m, nil
			}
			if account.Email != m.cfg.Account || account.Client != m.cfg.Client {
				m.selected = make(map[string]bool)
			}
			m.cfg.Account, m.cfg.Client = account.Email, account.Client
			m.step, m.cursor = setupCalendar, 0
			cmd := m.discoverCalendars()
			return m, cmd
		}
		m.step = setupQuery
		cmd := m.query.Focus()
		return m, cmd
	}
	return m, nil
}

func (m setupModel) save() (setupModel, tea.Cmd) {
	cfg := m.cfg
	cfg.MailQuery = strings.TrimSpace(m.query.Value())
	cfg.Calendars = nil
	for _, calendar := range m.calendars {
		if m.selected[calendar.ID] {
			cfg.Calendars = append(cfg.Calendars, calendar.ID)
		}
	}
	cfg.Projects = nil
	for _, path := range strings.Split(m.projects.Value(), ";") {
		if path = strings.TrimSpace(path); path != "" {
			absolute, err := filepath.Abs(path)
			if err != nil {
				m.err = fmt.Errorf("project directory: %w", err)
				return m, nil
			}
			cfg.Projects = append(cfg.Projects, absolute)
		}
	}
	if m.preview {
		return m, func() tea.Msg { return setupSavedMsg{cfg: cfg} }
	}
	if err := config.Save(m.path, cfg); err != nil {
		m.err = err
		return m, nil
	}
	return m, func() tea.Msg { return setupSavedMsg{cfg: cfg} }
}

func (m setupModel) View(width, height int) string {
	if width < 24 || height < 8 {
		return "Enlarge pane for setup. Esc cancels."
	}
	title := "guhd setup"
	if m.preview {
		title = "guhd setup preview (no saves)"
	}
	lines := []string{title, ""}
	inputRow := -1
	switch m.step {
	case setupAccount, setupCalendar:
		label := "Choose an account"
		if m.step == setupCalendar {
			label = "Choose calendars (Space toggles)"
		}
		lines = append(lines, label)
		if m.loading {
			lines = append(lines, "Loading…")
		} else {
			count := len(m.accounts)
			if m.step == setupCalendar {
				count = len(m.calendars)
			}
			rows := max(1, height-8)
			start := max(0, m.cursor-rows+1)
			for i := start; i < min(count, start+rows); i++ {
				prefix := "  "
				if i == m.cursor {
					prefix = "> "
				}
				if m.step == setupAccount {
					a := m.accounts[i]
					label := a.Email + " (" + a.Client + ")"
					if a.Error != "" {
						label += " · unavailable: " + a.Error
					}
					lines = append(lines, prefix+label)
				} else {
					c := m.calendars[i]
					mark := "[ ] "
					if m.selected[c.ID] {
						mark = "[x] "
					}
					lines = append(lines, prefix+mark+c.Name+" ("+c.ID+")")
				}
			}
			if count == 0 && m.err == nil {
				lines = append(lines, "No calendars available. Enter continues.")
			}
		}
	case setupQuery:
		inputRow = 3
		lines = append(lines, "Gmail query", m.query.View())
	case setupProjects:
		inputRow = 4
		lines = append(lines, "Project directories (optional)", "Separate paths with semicolons (;).", m.projects.View(), "Enter saves configuration.")
	}
	if m.err != nil {
		lines = append(lines, "Error: "+m.err.Error())
	}
	lines = append(lines, "", "Enter next · Shift+Tab back · Esc cancel")
	if m.step <= setupCalendar {
		lines = append(lines, "↑/↓ choose · r retry discovery")
	}
	for i, line := range lines {
		if i != inputRow {
			line = strings.NewReplacer("\n", " ", "\t", " ").Replace(plainText(line))
		}
		if i != inputRow {
			style := m.styles.Normal
			switch {
			case i == 0:
				style = m.styles.Heading
			case strings.HasPrefix(line, "> "):
				style = m.styles.Selected
			case strings.HasPrefix(line, "Error:"):
				style = m.styles.Error
			case strings.HasPrefix(line, "Enter ") || strings.HasPrefix(line, "↑/↓"):
				style = m.styles.Footer
			}
			line = styledLine(style, line, width)
		}
		lines[i] = ansi.Truncate(line, width, "…")
	}
	return strings.Join(lines, "\n")
}

// Sanitize values before rendering so trusted cursor styling remains visible.
func updateInput(input textinput.Model, msg tea.Msg) (textinput.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		v.Text = plainText(v.Text)
		msg = v
	case tea.PasteMsg:
		v.Content = plainText(v.Content)
		msg = v
	}
	input, cmd := input.Update(msg)
	value := input.Value()
	if clean := plainText(value); clean != value {
		position := len([]rune(plainText(string([]rune(value)[:input.Position()]))))
		input.SetValue(clean)
		input.SetCursor(position)
	}
	return input, cmd
}
