package dashboard

import (
	"errors"
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/cosgroveb/guhd/internal/termimage"
)

func TestThemedPreviewImages(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	for _, name := range []string{"plain", "moni-chrome"} {
		for _, size := range [][2]int{{20, 8}, {24, 10}, {71, 32}, {189, 51}} {
			m := previewModel(t, name)
			m.cfg.Images = "blocks"
			m.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m.section = 1
			if m.openDetail() != nil || m.client != nil || !m.hasImages() || m.detail.image.index != -1 {
				t.Fatal("preview must expose an unloaded fictional attachment without a client")
			}
			_, cmd := m.Update(key("i"))
			if cmd == nil || !m.detail.image.loading {
				t.Fatal("image key did not start loading")
			}
			m.Update(cmd())
			if m.detail.image.err != nil || m.detail.image.decoded == nil {
				t.Fatal("preview image failed", m.detail.image.err)
			}
			if !strings.Contains(assertBounds(t, m), "▀") {
				t.Fatal("preview image is not visible")
			}
			width, height := m.contentSize()
			if m.detail.image.cols > width || m.detail.image.rows > max(1, height-7) {
				t.Fatal("image exceeds themed content")
			}
			_, cmd = m.Update(key("i"))
			m.Update(cmd())
			if m.detail.image.index != 0 {
				t.Fatal("single image cycle changed selection")
			}
			m.Update(key("esc"))
			m.selected[1] = 1
			m.openDetail()
			if m.hasImages() {
				t.Fatal("non-image preview mail has attachments")
			}
		}
	}
}

func TestThemeBackgroundAndNativeImageIdentity(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("TMUX", "")
	m := previewModel(t, "moni-chrome")
	m.Update(tea.WindowSizeMsg{Width: 71, Height: 32})
	m.profile = colorprofile.TrueColor
	m.cfg.Images = "blocks"
	m.section = 1
	m.openDetail()
	m.detail.image.index = 0
	m.detail.image.decoded = image.NewNRGBA(image.Rect(0, 0, 1, 2))
	m.prepareImage()
	if !strings.Contains(m.detail.image.grid, "38;2;5;3;1") || !strings.Contains(m.detail.image.grid, "48;2;5;3;1") {
		t.Fatal("transparent thumbnail did not use moni-chrome background")
	}
	m.cfg.Images = "kitty"
	m.prepareImage()
	v := &m.detail.image
	_, grid, err := termimage.Native(v.decoded, v.id, v.cols, v.rows, false)
	if err != nil {
		t.Fatal(err)
	}
	v.reveal = true
	m.imageReady(imageReadyMsg{detail: m.detail.request, request: v.request, id: v.id, grid: grid})
	// The viewport and outer frame must preserve the protocol's RGB image ID.
	firstLine, _, _ := strings.Cut(grid, "\n")
	if !strings.Contains(m.detail.viewport.View(), firstLine) || !strings.Contains(m.View().Content, firstLine) {
		t.Fatal("theme changed native placeholder foreground")
	}
	assertBounds(t, m)
}

func TestImageTextThemeRoles(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	for _, name := range []string{"plain", "moni-chrome"} {
		m := previewModel(t, name)
		m.section = 1
		m.openDetail()
		d := m.detail
		d.image.index = 0
		blank := ""
		if name != "plain" {
			blank = m.styles.Normal.Render(" ")
		}
		caption := "\n" + blank + "\n" + m.styles.Muted.Render("Image 1/1: trail.png")
		d.image.loading = true
		if got, want := m.imageText(), caption+m.styles.Muted.Render(" · loading"); got != want {
			t.Fatalf("loading text: %q, want %q", got, want)
		}
		d.image.loading = false
		d.image.err = errors.New("broken\x1b[31m")
		if got, want := m.imageText(), caption+m.styles.Error.Render(" · broken"); got != want {
			t.Fatalf("error text: %q, want %q", got, want)
		}
		d.image.err = nil
		_, data := previewImage()
		decoded, err := termimage.Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		_, grid, err := termimage.Native(decoded, 0x123456, 4, 2, false)
		if err != nil {
			t.Fatal(err)
		}
		d.image.grid = grid
		if got, want := m.imageText(), caption+"\n"+grid; got != want {
			t.Fatal("caption styling changed native grid")
		}
		d.image.index = -1
		d.attachments[0].Unavailable = "too large"
		if got, want := m.imageText(), "\n"+m.styles.Warning.Render("Image trail.png unavailable: too large"); got != want {
			t.Fatalf("unavailable text: %q, want %q", got, want)
		}
	}
}

func TestImageTextFillsThemeBackground(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	m := previewModel(t, "moni-chrome")
	m.Update(tea.WindowSizeMsg{Width: 71, Height: 32})
	m.section = 1
	m.openDetail()
	m.detail.image.index = 0
	m.detail.image.loading = true
	width, _ := m.contentSize()
	lines := strings.Split(m.imageText(), "\n")
	for _, line := range lines[1:] {
		if ansi.StringWidth(line) != width || !strings.Contains(line, "48;2;5;3;1") {
			t.Fatalf("image preamble row lacks full theme background: %q", line)
		}
	}
}
