package dashboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/cosgroveb/guhd/internal/source"
	"github.com/cosgroveb/guhd/internal/termimage"
	"github.com/cosgroveb/guhd/internal/theme"
)

type imageSource struct {
	fixtureSource
	calls       int
	data        []byte
	err         error
	attachments []source.Attachment
}

func (s *imageSource) Detail(_ context.Context, _ source.Account, id string) (source.MessageDetail, error) {
	return source.MessageDetail{Message: source.Message{ID: id}, Body: strings.Repeat("long body\n", 200), Attachments: s.attachments}, nil
}
func (s *imageSource) Attachment(ctx context.Context, _ source.Account, _ string, _ source.Attachment) ([]byte, error) {
	s.calls++
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return s.data, s.err
}
func imageModel(t *testing.T) (*model, *imageSource) {
	t.Helper()
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	m := testModel(t)
	m.profile = colorprofile.TrueColor
	m.cfg.Images = "blocks"
	a, data := previewImage()
	b := a
	b.Name = "next.png"
	b.ID = "next"
	s := &imageSource{data: data, attachments: []source.Attachment{a, b}}
	m.client = s
	m.section = 1
	m.mail = []source.Message{{ID: "mail", Subject: "A view from the trail"}}
	m.Update(m.openDetail()())
	return m, s
}
func TestImagesExplicitCycleAndVisibility(t *testing.T) {
	m, s := imageModel(t)
	if s.calls != 0 || strings.Contains(m.detail.viewport.View(), "Image 1/") {
		t.Fatal("opening mail fetched or displayed image")
	}
	cmd := m.nextImage()
	m.Update(cmd())
	if s.calls != 1 || !strings.Contains(ansi.Strip(m.detail.viewport.View()), "▀") {
		t.Fatalf("image not revealed: %q", m.detail.viewport.View())
	}
	for _, want := range []int{1, 0} {
		cmd = m.nextImage()
		m.Update(cmd())
		if m.detail.image.index != want {
			t.Fatal("wrong image index")
		}
	}
}
func TestImageStaleFailureCancellationAndOff(t *testing.T) {
	m, _ := imageModel(t)
	cmd := m.nextImage()
	old := cmd()
	m.nextImage()
	m.Update(old)
	if m.detail.image.decoded != nil {
		t.Fatal("stale result accepted")
	}
	cmd = m.nextImage()
	m.Update(key("esc"))
	msg := cmd().(imageMsg)
	if !errors.Is(msg.err, context.Canceled) {
		t.Fatalf("not canceled: %v", msg.err)
	}
	m.Update(msg)
	if m.detail != nil {
		t.Fatal("stale image reopened detail")
	}
	m, s := imageModel(t)
	s.err = errors.New("broken\x1b[31m")
	m.Update(m.nextImage()())
	if !strings.Contains(m.detail.viewport.View(), "broken") || strings.Contains(m.detail.viewport.View(), "[31m") {
		t.Fatal("unsafe error")
	}
	m.cfg.Images = "off"
	if m.nextImage() != nil {
		t.Fatal("off requests image")
	}
}
func TestImageProfileAndNativeCleanup(t *testing.T) {
	m, _ := imageModel(t)
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("TMUX", "")
	m.cfg.Images = "auto"
	if !m.nativeImages() {
		t.Fatal("direct kitty not selected")
	}
	t.Setenv("TMUX", "session")
	if m.nativeImages() {
		t.Fatal("auto selected tmux kitty")
	}
	m.cfg.Images = "kitty"
	if !m.nativeImages() {
		t.Fatal("explicit kitty not selected")
	}
	t.Setenv("NO_COLOR", "1")
	if m.nativeImages() || m.imageProfile() != colorprofile.ASCII {
		t.Fatal("NO_COLOR ignored")
	}
	t.Setenv("NO_COLOR", "")
	m.profile = colorprofile.ANSI256
	if m.nativeImages() {
		t.Fatal("native without truecolor")
	}
	m.ownedImages = map[uint32]ownedImage{123: {}, 456: {tmux: true}}
	var out bytes.Buffer
	if err := CleanupImages(m, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != termimage.Delete(123, false)+termimage.Delete(456, true) && out.String() != termimage.Delete(456, true)+termimage.Delete(123, false) {
		t.Fatal("cleanup deleted wrong IDs")
	}
	m.detail = nil
	if m.imageReady(imageReadyMsg{id: 123}) == nil {
		t.Fatal("stale upload not deleted")
	}
}
func TestImageGeometryAndSanitation(t *testing.T) {
	for _, size := range [][2]int{{20, 8}, {71, 25}, {71, 32}, {189, 32}} {
		m, s := imageModel(t)
		s.attachments[0].Name = "evil\x1b[31m.png"
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.Update(m.nextImage()())
		view := m.View().Content
		lines := strings.Split(view, "\n")
		if len(lines) > size[1] {
			t.Fatal("too tall")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("too wide")
			}
		}
		m.detail.viewport.GotoTop()
		m.Update(tea.WindowSizeMsg{Width: size[0] + 1, Height: size[1]})
		if m.detail.viewport.YOffset() != 0 {
			t.Fatal("resize moved scroll")
		}
	}
}

func TestNativeReadyResizeAndDowngrade(t *testing.T) {
	m, _ := imageModel(t)
	m.cfg.Images = "kitty"
	_, data := previewImage()
	decoded, err := termimage.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	m.detail.image.decoded = decoded
	m.detail.image.index = 0
	if m.prepareImage() == nil || m.detail.image.id == 0 {
		t.Fatal("no native upload")
	}
	id := m.detail.image.id
	if _, ok := m.ownedImages[id]; !ok {
		t.Fatal("upload not registered")
	}
	m.detail.image.reveal = true
	m.imageReady(imageReadyMsg{detail: m.detail.request, request: m.detail.image.request, id: id, grid: "native image"})
	if !strings.Contains(m.detail.viewport.View(), "native image") {
		t.Fatal("ready image not revealed")
	}
	m.detail.viewport.GotoTop()
	m.Update(tea.WindowSizeMsg{Width: 71, Height: 25})
	next := m.detail.image.id
	if next == id || next == 0 {
		t.Fatal("resize did not replace placement")
	}
	m.imageReady(imageReadyMsg{detail: m.detail.request, request: m.detail.image.request, id: next, grid: "new image"})
	if m.detail.viewport.YOffset() != 0 {
		t.Fatal("native resize moved scroll")
	}
	if m.imageReady(imageReadyMsg{detail: m.detail.request, request: m.detail.image.request, id: id, grid: "old image"}) == nil {
		t.Fatal("stale resized upload not deleted")
	}
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	if m.detail.image.id != 0 || strings.Contains(m.detail.image.grid, "\U0010eeee") {
		t.Fatal("downgrade kept native placeholder")
	}
}

func TestImageProfileSurvivesSetup(t *testing.T) {
	m, s := imageModel(t)
	m.profile = colorprofile.Unknown
	setup := newSetup(m.ctx, m.cfg, "", m.client, theme.Styles{})
	m.setup = &setup
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	m.Update(setupSavedMsg{cfg: m.cfg})
	if m.setup != nil || m.profile != colorprofile.TrueColor {
		t.Fatal("setup discarded color profile")
	}
	m.Update(m.openDetail()())
	m.Update(m.nextImage()())
	if s.calls != 1 || !strings.Contains(m.detail.image.grid, "\x1b[38;2;") {
		t.Fatal("first image after setup has no truecolor")
	}
}

// Run real sequences so retirement must follow Raw processing and shutdown flushes.
type nativeCycleModel struct {
	*model
	cycles int
	err    string
}

type nativeCycleMsg struct{}

func (m *nativeCycleModel) Init() tea.Cmd {
	return func() tea.Msg { return nativeCycleMsg{} }
}
func (m *nativeCycleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.ColorProfileMsg, tea.WindowSizeMsg:
		return m, nil
	case nativeCycleMsg:
		if len(m.ownedImages) != 0 {
			m.err = "completed cycle retained image ownership"
			return m, tea.Quit
		}
		if m.cycles == 100 {
			return m, tea.Quit
		}
		m.cycles++
		return m, m.prepareImage()
	case imageReadyMsg:
		m.model.Update(msg)
		return m, tea.Sequence(m.releaseImage(), m.Init())
	default:
		_, cmd := m.model.Update(msg)
		return m, cmd
	}
}

func TestNativeCompletedCyclesRetireOwnership(t *testing.T) {
	m, _ := imageModel(t)
	m.cfg.Images = "kitty"
	m.preview = true
	_, data := previewImage()
	decoded, err := termimage.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	m.detail.image.decoded = decoded
	var output bytes.Buffer
	cycles := &nativeCycleModel{model: m}
	p := tea.NewProgram(cycles, tea.WithInput(nil), tea.WithOutput(&output), tea.WithoutRenderer())
	if _, err := p.Run(); err != nil {
		t.Fatal(err)
	}
	if cycles.err != "" {
		t.Fatal(cycles.err)
	}
	if cycles.cycles != 100 || len(m.ownedImages) != 0 {
		t.Fatalf("cycles=%d, owned=%d", cycles.cycles, len(m.ownedImages))
	}
	if strings.Count(output.String(), "a=d") != 100 {
		t.Fatal("shutdown did not flush queued deletions")
	}
}

type nativeStepsModel struct {
	*model
	steps []func() tea.Cmd
}

type nativeStepMsg struct{}

func (m *nativeStepsModel) Init() tea.Cmd {
	return func() tea.Msg { return nativeStepMsg{} }
}
func (m *nativeStepsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.ColorProfileMsg, tea.WindowSizeMsg:
		return m, nil
	case nativeStepMsg:
		if len(m.steps) == 0 {
			return m, tea.Quit
		}
		step := m.steps[0]
		m.steps = m.steps[1:]
		return m, tea.Sequence(step(), m.Init())
	default:
		_, cmd := m.model.Update(msg)
		return m, cmd
	}
}

func TestNativeReplacementBackAndLateUpload(t *testing.T) {
	m, _ := imageModel(t)
	m.cfg.Images = "kitty"
	m.preview = true
	_, data := previewImage()
	decoded, err := termimage.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	m.detail.image.decoded = decoded
	var first, second uint32
	var firstReady imageReadyMsg
	steps := &nativeStepsModel{model: m, steps: []func() tea.Cmd{
		func() tea.Cmd {
			m.prepareImage()
			first = m.detail.image.id
			firstReady = imageReadyMsg{detail: m.detail.request, request: m.detail.image.request, id: first}
			return m.releaseImage()
		},
		func() tea.Cmd {
			if _, ok := m.ownedImages[first]; !ok {
				t.Error("early deletion retired pending upload")
			}
			m.prepareImage()
			second = m.detail.image.id
			// The old upload may have passed its cancellation check before release.
			upload, _, err := termimage.Native(decoded, first, 1, 1, false)
			if err != nil {
				t.Fatal(err)
			}
			return tea.Raw(upload)
		},
		func() tea.Cmd { return m.imageReady(firstReady) },
		func() tea.Cmd {
			if _, ok := m.ownedImages[first]; ok {
				t.Error("stale completed upload still owned")
			}
			if len(m.ownedImages) != 1 {
				t.Errorf("replacement ownership: %d", len(m.ownedImages))
			}
			_, cmd := m.Update(key("esc"))
			return cmd
		},
	}}
	var output bytes.Buffer
	p := tea.NewProgram(steps, tea.WithInput(nil), tea.WithOutput(&output), tea.WithoutRenderer())
	if _, err := p.Run(); err != nil {
		t.Fatal(err)
	}
	if m.detail != nil || len(m.ownedImages) != 1 {
		t.Fatal("back lost pending upload ownership")
	}
	var cleanup bytes.Buffer
	if err := CleanupImages(m, &cleanup); err != nil {
		t.Fatal(err)
	}
	if cleanup.String() != termimage.Delete(second, m.ownedImages[second].tmux) {
		t.Fatal("quit did not delete pending replacement")
	}
	if strings.Count(output.String(), "i="+fmt.Sprint(first)+",") < 2 {
		t.Fatal("late upload and stale deletion were not flushed")
	}
}

func TestNativeQuitBeforeCleanupDispatch(t *testing.T) {
	for _, ready := range []bool{false, true} {
		t.Run(fmt.Sprint(ready), func(t *testing.T) {
			m, _ := imageModel(t)
			m.cfg.Images = "kitty"
			_, data := previewImage()
			decoded, err := termimage.Decode(data)
			if err != nil {
				t.Fatal(err)
			}
			m.detail.image.decoded = decoded
			m.prepareImage()
			id := m.detail.image.id
			if ready {
				m.imageReady(imageReadyMsg{detail: m.detail.request, request: m.detail.image.request, id: id})
			}
			m.releaseImage() // Quit can discard this command before Raw is processed.
			var output bytes.Buffer
			if err := CleanupImages(m, &output); err != nil {
				t.Fatal(err)
			}
			if output.String() != termimage.Delete(id, m.ownedImages[id].tmux) {
				t.Fatal("quit lost an undispatched deletion")
			}
		})
	}
}
