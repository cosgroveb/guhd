package dashboard

import (
	"context"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/cosgroveb/guhd/internal/config"
	"github.com/cosgroveb/guhd/internal/source"
	"github.com/cosgroveb/guhd/internal/theme"
)

type dataSource interface {
	Attachment(context.Context, source.Account, string, source.Attachment) ([]byte, error)
	Accounts(context.Context) ([]source.Account, error)
	Calendars(context.Context, source.Account) ([]source.Calendar, error)
	Events(context.Context, source.Account, []string, time.Time) ([]source.Event, error)
	Messages(context.Context, source.Account, string, string) (source.MailPage, error)
	Counts(context.Context, source.Account) (source.MailCounts, error)
	Detail(context.Context, source.Account, string) (source.MessageDetail, error)
}
type refreshState struct {
	loading           bool
	success, timeNext time.Time
	err               error
}
type eventsMsg struct {
	rows []source.Event
	err  error
}
type mailMsg struct {
	page   source.MailPage
	append bool
	err    error
}
type countsMsg struct {
	counts source.MailCounts
	err    error
}
type projectsMsg struct {
	rows []source.Project
	err  error
}
type detailMsg struct {
	request uint64
	id      string
	detail  source.MessageDetail
	err     error
}
type clockMsg time.Time
type actionMsg struct{ err error }
type detailView struct {
	cancel                  context.CancelFunc
	attachments             []source.Attachment
	image                   imageView
	id, title, text, target string
	local                   bool
	loading                 bool
	request                 uint64
	viewport                viewport.Model
}
type model struct {
	profile                colorprofile.Profile
	ownedImages            map[uint32]ownedImage
	ctx                    context.Context
	cancel                 context.CancelFunc
	cfg                    config.Config
	path                   string
	client                 dataSource
	setup                  *setupModel
	width, height, section int
	selected               [3]int
	events                 []source.Event
	mail                   []source.Message
	projects               []source.Project
	nextPage               string
	counts                 source.MailCounts
	states                 [4]refreshState
	now                    time.Time
	detail                 *detailView
	request                uint64
	help                   bool
	status                 string
	styles                 theme.Styles
	preview                bool
}

// New creates a dashboard whose fetches share the application's lifetime.
func New(ctx context.Context, cfg config.Config, path string, setup bool, styles theme.Styles) tea.Model {
	ctx, cancel := context.WithCancel(ctx)
	m := &model{ctx: ctx, cancel: cancel, cfg: cfg, path: path, client: source.Gog{}, now: time.Now(), styles: styles}
	m.focusInitialSection()
	if setup {
		s := newSetup(ctx, cfg, path, m.client, m.styles)
		m.setup = &s
	}
	return m
}
func (m *model) focusInitialSection() {
	m.section = 0
	if len(m.cfg.Calendars) == 0 {
		m.section = 1
	}
}
func (m *model) Init() tea.Cmd {
	if m.preview {
		return nil
	}
	if m.setup != nil {
		return m.setup.Init()
	}
	return tea.Batch(m.refresh(false), tick())
}
func tick() tea.Cmd { return tea.Tick(time.Second, func(t time.Time) tea.Msg { return clockMsg(t) }) }
func (m *model) account() source.Account {
	return source.Account{Email: m.cfg.Account, Client: m.cfg.Client}
}
func (m *model) refresh(force bool) tea.Cmd {
	if m.preview {
		return nil
	}
	var cmds []tea.Cmd
	for i := range m.states {
		if i == 2 && len(m.cfg.Projects) == 0 || i == 3 && m.cfg.MailQuery != "in:inbox" {
			continue
		}
		s := &m.states[i]
		if s.loading || (!force && !s.timeNext.IsZero() && m.now.Before(s.timeNext)) {
			continue
		}
		s.loading = true
		ctx, client, account, cfg, now := m.ctx, m.client, m.account(), m.cfg, m.now
		switch i {
		case 0:
			cmds = append(cmds, func() tea.Msg {
				rows, err := client.Events(ctx, account, cfg.Calendars, now)
				return eventsMsg{rows, err}
			})
		case 1:
			cmds = append(cmds, func() tea.Msg {
				page, err := client.Messages(ctx, account, cfg.MailQuery, "")
				return mailMsg{page: page, err: err}
			})
		case 2:
			cmds = append(cmds, func() tea.Msg { rows, err := source.Projects(ctx, cfg.Projects); return projectsMsg{rows, err} })
		case 3:
			cmds = append(cmds, func() tea.Msg { counts, err := client.Counts(ctx, account); return countsMsg{counts, err} })
		}
	}
	return tea.Batch(cmds...)
}
func (m *model) completed(i int, err error) {
	s := &m.states[i]
	s.loading = false
	s.err = err
	s.timeNext = m.now.Add(time.Duration(max(1, m.cfg.RefreshSeconds)) * time.Second)
	if err == nil {
		s.success = m.now
	}
}
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if profile, ok := msg.(tea.ColorProfileMsg); ok {
		m.profile = profile.Profile
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
		m.cancel()
		return m, tea.Quit
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		m.resizeDetail()
		width, height := m.contentSize()
		msg = tea.WindowSizeMsg{Width: width, Height: height}
		if m.setup == nil {
			return m, m.prepareImage()
		}
	}
	if m.preview {
		if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "s" && m.setup == nil && m.detail == nil && !m.help {
			m.previewSetup()
			return m, nil
		}
	}
	if m.setup != nil {
		switch v := msg.(type) {
		case setupCanceledMsg:
			if m.preview {
				m.setup = nil
				return m, nil
			}
			m.cancel()
			return m, tea.Quit
		case setupSavedMsg:
			if m.preview {
				m.setup = nil
				return m, nil
			}
			m.cfg = v.cfg
			m.focusInitialSection()
			m.setup = nil
			return m, tea.Batch(m.refresh(false), tick())
		default:
			s, cmd := m.setup.Update(msg)
			m.setup = &s
			return m, cmd
		}
	}
	switch v := msg.(type) {
	case tea.ColorProfileMsg:
		return m, m.prepareImage()
	case imageMsg:
		return m, m.imageLoaded(v)
	case imageDeletedMsg:
		delete(m.ownedImages, uint32(v))
		return m, nil
	case imageReadyMsg:
		return m, m.imageReady(v)
	case clockMsg:
		if m.preview {
			return m, nil
		}
		m.now = time.Time(v)
		m.expireEvents()
		return m, tea.Batch(m.refresh(false), tick())
	case eventsMsg:
		m.completed(0, v.err)
		if v.err == nil {
			id := m.selectedID(0)
			m.events = v.rows
			m.restore(0, id)
			m.expireEvents()
		}
	case mailMsg:
		m.completed(1, v.err)
		if v.err == nil {
			id := m.selectedID(1)
			if v.append {
				m.mail = append(m.mail, v.page.Messages...)
			} else {
				m.mail = v.page.Messages
			}
			m.nextPage = v.page.NextPageToken
			m.restore(1, id)
		}
	case countsMsg:
		m.completed(3, v.err)
		if v.err == nil {
			m.counts = v.counts
		}
	case projectsMsg:
		m.completed(2, v.err)
		if v.err == nil {
			id := m.selectedID(2)
			m.projects = v.rows
			m.restore(2, id)
		}
	case detailMsg:
		if d := m.detail; d != nil && d.loading && d.request == v.request && d.id == v.id {
			d.loading = false
			if v.err != nil {
				d.text = "Unable to load message: " + v.err.Error()
			} else {
				d.text = messageText(v.detail)
				d.attachments = v.detail.Attachments
				d.image.index = -1
			}
			m.resizeDetail()
		}
	case actionMsg:
		if v.err != nil {
			m.status = v.err.Error()
		} else {
			m.status = "Opened"
		}
	case tea.KeyPressMsg:
		switch v.String() {
		case "q":
			m.cancel()
			return m, tea.Quit
		case "?":
			m.help = !m.help
		case "esc":
			cleanup := m.closeDetail()
			m.detail = nil
			m.help = false
			m.status = ""
			return m, cleanup
		case "i":
			if !m.help {
				return m, m.nextImage()
			}
		case "r":
			return m, m.refresh(true)
		case "o", "y":
			return m, m.action(v.String())
		default:
			if m.detail != nil {
				var cmd tea.Cmd
				m.detail.viewport, cmd = m.detail.viewport.Update(msg)
				return m, cmd
			}
			switch v.String() {
			case "tab":
				m.section = (m.section + 1) % m.sectionCount()
			case "shift+tab":
				m.section = (m.section + m.sectionCount() - 1) % m.sectionCount()
			case "down", "j":
				m.selected[m.section] = min(m.length(m.section)-1, m.selected[m.section]+1)
				m.selected[m.section] = max(0, m.selected[m.section])
			case "up", "k":
				m.selected[m.section] = max(0, m.selected[m.section]-1)
			case "enter":
				return m, m.openDetail()
			case "n":
				if m.preview {
					return m, nil
				}
				if m.section == 1 && m.nextPage != "" && !m.states[1].loading {
					m.states[1].loading = true
					ctx, client, a, q, p := m.ctx, m.client, m.account(), m.cfg.MailQuery, m.nextPage
					return m, func() tea.Msg {
						page, err := client.Messages(ctx, a, q, p)
						return mailMsg{page: page, append: true, err: err}
					}
				}
			}
		}
	}
	return m, nil
}
func (m *model) sectionCount() int {
	if len(m.cfg.Projects) > 0 {
		return 3
	}
	return 2
}
func (m *model) length(section int) int {
	switch section {
	case 0:
		return len(m.events)
	case 1:
		return len(m.mail)
	default:
		return len(m.projects)
	}
}
func (m *model) selectedID(section int) string {
	n := m.selected[section]
	if n < 0 || n >= m.length(section) {
		return ""
	}
	switch section {
	case 0:
		return m.events[n].Key()
	case 1:
		return m.mail[n].ID
	default:
		return m.projects[n].Path
	}
}
func (m *model) restore(section int, id string) {
	for n := 0; n < m.length(section); n++ {
		old := m.selected[section]
		m.selected[section] = n
		if m.selectedID(section) == id {
			return
		}
		m.selected[section] = old
	}
	m.selected[section] = max(0, min(m.selected[section], m.length(section)-1))
}
func (m *model) expireEvents() {
	id := m.selectedID(0)
	rows := make([]source.Event, 0, len(m.events))
	for _, e := range m.events {
		if e.End.After(m.now) {
			rows = append(rows, e)
		}
	}
	m.events = rows
	m.restore(0, id)
}
