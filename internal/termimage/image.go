// Package termimage renders bounded, static attachment images without terminal I/O.
package termimage

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// Decode checks dimensions before allocating pixels and retains at most 1024 per side.
// GIF decoding reads only its first frame.
func Decode(data []byte) (image.Image, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read image dimensions: %w", err)
	}
	if format != "png" && format != "jpeg" && format != "gif" {
		return nil, fmt.Errorf("unsupported image format %q", format)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 8_000_000 {
		return nil, fmt.Errorf("image exceeds 4096 pixels per side or 8 million pixels")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	if max(cfg.Width, cfg.Height) <= 1024 {
		return img, nil
	}
	scale := 1024.0 / float64(max(cfg.Width, cfg.Height))
	width, height := max(1, int(float64(cfg.Width)*scale)), max(1, int(float64(cfg.Height)*scale))
	resized := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			resized.Set(x, y, sample(img, x, y, width, height))
		}
	}
	return resized, nil
}

// Size fits an image in cells, assuming cells are twice as tall as they are wide.
// Output is capped at 120 columns and 60 rows.
func Size(img image.Image, width, height int) (cols, rows int) {
	if img == nil || width <= 0 || height <= 0 || img.Bounds().Empty() {
		return 0, 0
	}
	b := img.Bounds()
	scale := min(float64(min(width, 120))/float64(b.Dx()), float64(min(height, 60)*2)/float64(b.Dy()))
	return max(1, int(math.Round(float64(b.Dx())*scale))), max(1, int(math.Round(float64(b.Dy())*scale/2)))
}

// Blocks draws two sampled pixels per cell. A nil background uses neutral #101010.
func Blocks(img image.Image, cols, rows int, background color.Color, profile colorprofile.Profile) string {
	if img == nil || img.Bounds().Empty() || cols <= 0 || rows <= 0 {
		return ""
	}
	if profile <= colorprofile.ASCII {
		return "[image: color terminal required]"
	}
	cols, rows = min(cols, 120), min(rows, 60)
	if background == nil {
		background = color.RGBA{R: 16, G: 16, B: 16, A: 255}
	}
	var out strings.Builder
	for y := 0; y < rows; y++ {
		if y > 0 {
			out.WriteByte('\n')
		}
		for x := 0; x < cols; x++ {
			top := composite(sample(img, x, y*2, cols, rows*2), background)
			bottom := composite(sample(img, x, y*2+1, cols, rows*2), background)
			out.WriteString(ansi.Style{}.ForegroundColor(convert(top, profile)).BackgroundColor(convert(bottom, profile)).Styled("▀"))
		}
	}
	return out.String()
}

func sample(img image.Image, x, y, width, height int) color.Color {
	b := img.Bounds()
	return img.At(b.Min.X+min(b.Dx()-1, (2*x+1)*b.Dx()/(2*width)), b.Min.Y+min(b.Dy()-1, (2*y+1)*b.Dy()/(2*height)))
}

func composite(fg, bg color.Color) color.Color {
	r, g, b, a := fg.RGBA()
	br, bgGreen, bb, _ := bg.RGBA()
	return color.RGBA{R: uint8((r + br*(65535-a)/65535) >> 8), G: uint8((g + bgGreen*(65535-a)/65535) >> 8), B: uint8((b + bb*(65535-a)/65535) >> 8), A: 255}
}

func convert(c color.Color, profile colorprofile.Profile) color.Color {
	if profile == colorprofile.ANSI {
		return ansi.Convert16(c)
	}
	if profile == colorprofile.ANSI256 {
		return ansi.Convert256(c)
	}
	return c
}

// Native returns a direct Kitty upload and a Unicode placeholder grid. The caller
// owns the nonzero 24-bit ID and must send upload outside the text renderer.
func Native(img image.Image, id uint32, cols, rows int, tmux bool) (upload, grid string, err error) {
	if img == nil || img.Bounds().Empty() || id == 0 || id > 0xffffff || cols < 1 || rows < 1 || cols > 120 || rows > 60 {
		return "", "", fmt.Errorf("invalid native image, ID or dimensions")
	}
	opts := kitty.Options{Action: kitty.TransmitAndPut, Transmission: kitty.Direct, Format: kitty.PNG, ID: int(id), Columns: cols, Rows: rows, VirtualPlacement: true, DoNotMoveCursor: true, Quiet: 2, Chunk: true}
	if tmux {
		opts.ChunkFormatter = ansi.TmuxPassthrough
	}
	var data bytes.Buffer
	if err := kitty.EncodeGraphics(&data, img, &opts); err != nil {
		return "", "", fmt.Errorf("encode terminal image: %w", err)
	}
	var out strings.Builder
	for y := 0; y < rows; y++ {
		if y > 0 {
			out.WriteByte('\n')
		}
		fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%dm", id>>16, (id>>8)&255, id&255)
		for x := 0; x < cols; x++ {
			out.WriteRune(kitty.Placeholder)
			out.WriteRune(kitty.Diacritic(y))
			out.WriteRune(kitty.Diacritic(x))
		}
		out.WriteString("\x1b[39m")
	}
	return data.String(), out.String(), nil
}

// Delete removes only the owned image and its resource bytes.
func Delete(id uint32, tmux bool) string {
	if id == 0 || id > 0xffffff {
		return ""
	}
	opts := kitty.Options{Action: kitty.Delete, ID: int(id), Delete: kitty.DeleteID, DeleteResources: true, Quiet: 2}
	seq := ansi.KittyGraphics(nil, opts.Options()...)
	if tmux {
		return ansi.TmuxPassthrough(seq)
	}
	return seq
}
