package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/cosgroveb/guhd/internal/termimage"
)

type ownedImage struct {
	tmux, ready bool
}

type imageDeletedMsg uint32

type imageView struct {
	index      int
	request    uint64
	cancel     context.CancelFunc
	loading    bool
	reveal     bool
	err        error
	decoded    image.Image
	grid       string
	id         uint32
	cols, rows int
}
type imageMsg struct {
	detail, request uint64
	decoded         image.Image
	err             error
}
type imageReadyMsg struct {
	detail, request uint64
	id              uint32
	grid            string
}

func (m *model) hasImages() bool {
	if m.detail == nil {
		return false
	}
	for _, a := range m.detail.attachments {
		if a.Unavailable == "" {
			return true
		}
	}
	return false
}
func (m *model) nextImage() tea.Cmd {
	d := m.detail
	if d == nil || d.loading || !m.hasImages() || m.cfg.Images == "off" {
		return nil
	}
	old := m.releaseImage()
	v := &d.image
	v.request++
	v.index = (v.index + 1) % len(d.attachments)
	for d.attachments[v.index].Unavailable != "" {
		v.index = (v.index + 1) % len(d.attachments)
	}
	v.loading, v.err, v.decoded, v.grid = true, nil, nil, ""
	v.reveal = true
	ctx, cancel := context.WithCancel(m.ctx)
	v.cancel = cancel
	client, account, id, attachment, detail, request := m.client, m.account(), d.id, d.attachments[v.index], d.request, v.request
	m.resizeDetail()
	d.viewport.GotoBottom()
	preview := m.preview
	fetch := func() tea.Msg {
		var data []byte
		var err error
		if preview {
			_, data = previewImage()
		} else {
			data, err = client.Attachment(ctx, account, id, attachment)
		}
		var decoded image.Image
		if err == nil {
			decoded, err = termimage.Decode(data)
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return imageMsg{detail: detail, request: request, decoded: decoded, err: err}
	}
	return tea.Batch(old, fetch)
}
func (m *model) imageLoaded(msg imageMsg) tea.Cmd {
	d := m.detail
	if d == nil || d.request != msg.detail || d.image.request != msg.request {
		return nil
	}
	d.image.loading, d.image.decoded, d.image.err = false, msg.decoded, msg.err
	cmd := m.prepareImage()
	m.resizeDetail()
	d.viewport.GotoBottom()
	return cmd
}
func (m *model) nativeImages() bool {
	if m.profile != colorprofile.TrueColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if m.cfg.Images == "kitty" {
		return true
	}
	return (m.cfg.Images == "" || m.cfg.Images == "auto") && os.Getenv("TMUX") == "" && (os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "")
}
func (m *model) imageProfile() colorprofile.Profile {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return colorprofile.ASCII
	}
	return m.profile
}
func (m *model) prepareImage() tea.Cmd {
	d := m.detail
	if d == nil || d.image.decoded == nil {
		return nil
	}
	v := &d.image
	width, height := m.contentSize()
	cols, rows := termimage.Size(v.decoded, max(1, width), max(1, height-7))
	if m.nativeImages() && v.id != 0 && v.cols == cols && v.rows == rows {
		return nil
	}
	cleanup := m.releaseImage()
	v.cols, v.rows = cols, rows
	if !m.nativeImages() {
		var background color.Color
		if c := m.styles.Normal.GetBackground(); c != nil {
			if _, transparent := c.(lipgloss.NoColor); !transparent {
				background = c
			}
		}
		v.grid = termimage.Blocks(v.decoded, cols, rows, background, m.imageProfile())
		m.resizeDetail()
		return cleanup
	}
	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		v.err = err
		return cleanup
	}
	id := binary.BigEndian.Uint32(bytes[:]) & 0xffffff
	if m.ownedImages == nil {
		m.ownedImages = make(map[uint32]ownedImage)
	}
	for {
		_, exists := m.ownedImages[id]
		if id != 0 && !exists {
			break
		}
		id = (id + 1) & 0xffffff
	}
	upload, grid, err := termimage.Native(v.decoded, id, cols, rows, os.Getenv("TMUX") != "")
	if err != nil {
		v.err = err
		return cleanup
	}
	v.id, v.grid = id, ""
	// Register before dispatch: shutdown cleanup covers even uploads whose ready
	// message is discarded when the event loop exits.
	m.ownedImages[id] = ownedImage{tmux: os.Getenv("TMUX") != ""}
	detail, request := d.request, v.request
	ready := func() tea.Msg { return imageReadyMsg{detail: detail, request: request, id: id, grid: grid} }
	ctx, cancel := context.WithCancel(m.ctx)
	v.cancel = cancel
	write := func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		return tea.Raw(upload)()
	}
	return tea.Sequence(cleanup, write, ready)
}
func (m *model) imageReady(msg imageReadyMsg) tea.Cmd {
	owned, ok := m.ownedImages[msg.id]
	if !ok {
		return nil
	}
	owned.ready = true
	m.ownedImages[msg.id] = owned
	d := m.detail
	if d == nil || d.request != msg.detail || d.image.request != msg.request || d.image.id != msg.id {
		return m.deleteImage(msg.id)
	}
	d.image.grid = msg.grid
	m.resizeDetail()
	if d.image.reveal {
		d.viewport.GotoBottom()
		d.image.reveal = false
	}
	return nil
}
func (m *model) releaseImage() tea.Cmd {
	if m.detail == nil {
		return nil
	}
	v := &m.detail.image
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
	id := v.id
	v.id, v.grid = 0, ""
	if id == 0 {
		return nil
	}
	return m.deleteImage(id)
}
func (m *model) deleteImage(id uint32) tea.Cmd {
	owned := m.ownedImages[id]
	remove := tea.Raw(termimage.Delete(id, owned.tmux))
	if !owned.ready {
		// An upload can still arrive after this deletion. Its stale ready message
		// schedules the final deletion; until then shutdown retains ownership.
		return remove
	}
	// Bubble Tea queues Raw before the next message and flushes queued output
	// before Run returns, including on cancellation (v2.0.10).
	return tea.Sequence(remove, func() tea.Msg { return imageDeletedMsg(id) })
}
func (m *model) closeDetail() tea.Cmd {
	if m.detail == nil {
		return nil
	}
	if m.detail.cancel != nil {
		m.detail.cancel()
	}
	return m.releaseImage()
}
func (m *model) imageText() string {
	d := m.detail
	if d == nil || len(d.attachments) == 0 {
		return ""
	}
	width, _ := m.contentSize()
	pad := func(line string) string {
		if _, transparent := m.styles.Normal.GetBackground().(lipgloss.NoColor); !transparent && ansi.StringWidth(line) < width {
			line += m.styles.Normal.Render(strings.Repeat(" ", width-ansi.StringWidth(line)))
		}
		return line
	}
	var text strings.Builder
	for _, a := range d.attachments {
		if a.Unavailable != "" {
			text.WriteString("\n" + pad(m.styles.Warning.Render(fmt.Sprintf("Image %s unavailable: %s", oneLine(a.Name), oneLine(a.Unavailable)))))
		}
	}
	v := &d.image
	if v.index < 0 || v.index >= len(d.attachments) {
		return text.String()
	}
	caption := m.styles.Muted.Render(fmt.Sprintf("Image %d/%d: %s", v.index+1, len(d.attachments), oneLine(d.attachments[v.index].Name)))
	switch {
	case v.loading:
		caption += m.styles.Muted.Render(" · loading")
	case v.err != nil:
		caption += m.styles.Error.Render(" · " + oneLine(v.err.Error()))
	}
	text.WriteString("\n" + pad("") + "\n" + pad(caption))
	if !v.loading && v.err == nil && v.grid != "" {
		text.WriteString("\n" + v.grid)
	}
	return text.String()
}

// CleanupImages releases only this dashboard's image resources after Run stops.
// Raw commands alone cannot guarantee cleanup during terminal shutdown.
func CleanupImages(value tea.Model, output io.Writer) error {
	m, ok := value.(*model)
	if !ok {
		return nil
	}
	for id, owned := range m.ownedImages {
		if _, err := io.WriteString(output, termimage.Delete(id, owned.tmux)); err != nil {
			return err
		}
	}
	return nil
}
