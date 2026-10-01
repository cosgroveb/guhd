package termimage

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func TestDecodeFormatsAndReduction(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			img := image.NewRGBA(image.Rect(0, 0, 2048, 1024))
			var err error
			switch format {
			case "png":
				err = png.Encode(&buf, img)
			case "jpeg":
				err = jpeg.Encode(&buf, img, nil)
			case "gif":
				err = gif.Encode(&buf, img, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := Decode(buf.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Bounds() != image.Rect(0, 0, 1024, 512) {
				t.Fatal(decoded.Bounds())
			}
		})
	}
}

func TestDecodeRejectsDimensionsBeforePixels(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]uint32{{4097, 1}, {1, 4097}, {3000, 3000}} {
		data := bytes.Clone(buf.Bytes())
		binary.BigEndian.PutUint32(data[16:20], size[0])
		binary.BigEndian.PutUint32(data[20:24], size[1])
		binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
		if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("size %v: %v", size, err)
		}
	}
	for _, data := range [][]byte{nil, []byte("not an image"), buf.Bytes()[:40]} {
		if _, err := Decode(data); err == nil {
			t.Fatal("accepted corrupt image")
		}
	}
}

func TestSize(t *testing.T) {
	for _, tc := range []struct{ w, h, width, height, cols, rows int }{
		{200, 100, 100, 50, 100, 25}, {100, 200, 100, 50, 50, 50}, {100, 100, 999, 999, 120, 60}, {100, 100, 71, 25, 50, 25}, {100, 100, 20, 8, 16, 8}, {1, 4096, 1, 1, 1, 1}, {100, 100, 0, 10, 0, 0},
	} {
		cols, rows := Size(image.NewRGBA(image.Rect(0, 0, tc.w, tc.h)), tc.width, tc.height)
		if cols != tc.cols || rows != tc.rows {
			t.Fatalf("%+v: %d,%d", tc, cols, rows)
		}
	}
}

func TestBlocksColorAndAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(5, 5, 6, 7))
	img.SetNRGBA(5, 5, color.NRGBA{R: 255, A: 128})
	img.SetNRGBA(5, 6, color.NRGBA{})
	out := Blocks(img, 1, 1, color.RGBA{B: 255, A: 255}, colorprofile.TrueColor)
	if !strings.Contains(out, "38;2;128;0;127") || !strings.Contains(out, "48;2;0;0;255") || ansi.Strip(out) != "▀" {
		t.Fatalf("alpha: %q", out)
	}
	out = Blocks(img, 1, 1, nil, colorprofile.TrueColor)
	if !strings.Contains(out, "48;2;16;16;16") {
		t.Fatalf("neutral background: %q", out)
	}
	out = Blocks(img, 1, 1, nil, colorprofile.ANSI256)
	if strings.Contains(out, "38;2;") || !strings.Contains(out, "38;5;") {
		t.Fatalf("256: %q", out)
	}
	for _, profile := range []colorprofile.Profile{colorprofile.NoTTY, colorprofile.ASCII} {
		out = Blocks(img, 1, 1, nil, profile)
		if strings.Contains(out, "\x1b") || !strings.Contains(out, "color terminal") {
			t.Fatal(out)
		}
	}
}

func TestNativeTransportAndGrid(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	random := rand.New(rand.NewPCG(1, 2))
	for i := range img.Pix {
		img.Pix[i] = byte(random.Uint32())
	}
	upload, grid, err := Native(img, 0x123456, 2, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, param := range []string{"a=T", "f=100", "i=1193046", "U=1", "c=2", "r=2", "q=2"} {
		if !strings.Contains(upload, param) {
			t.Fatalf("missing %s", param)
		}
	}
	var payload strings.Builder
	chunks := strings.Split(strings.TrimSuffix(upload, "\x1b\\"), "\x1b\\")
	if len(chunks) < 2 {
		t.Fatal("fixture did not exercise chunking")
	}
	for i, chunk := range chunks {
		header, data, ok := strings.Cut(chunk, ";")
		if !ok || !strings.HasPrefix(header, "\x1b_G") || len(data) > 4096 || len(data)%4 != 0 {
			t.Fatalf("bad chunk %d", i)
		}
		more := "m=1"
		if i == len(chunks)-1 {
			more = "m=0"
		}
		if !strings.Contains(header, more) {
			t.Fatalf("chunk %d header: %s", i, header)
		}
		payload.WriteString(data)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(decoded)); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(grid, "\n")
	if len(lines) != 2 {
		t.Fatal(grid)
	}
	for y, line := range lines {
		if !strings.HasPrefix(line, "\x1b[38;2;18;52;86m") {
			t.Fatalf("ID: %q", line)
		}
		var want strings.Builder
		for x := 0; x < 2; x++ {
			want.WriteRune(kitty.Placeholder)
			want.WriteRune(kitty.Diacritic(y))
			want.WriteRune(kitty.Diacritic(x))
		}
		if ansi.Strip(line) != want.String() {
			t.Fatalf("grid: %q", line)
		}
	}
	wrapped, _, err := Native(img, 0x123456, 2, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(wrapped, "\x1bPtmux;") != len(chunks) || !strings.Contains(wrapped, "\x1b\x1b_G") {
		t.Fatal("each Kitty chunk must be independently wrapped")
	}
}

func TestDeleteAndInvalidIDs(t *testing.T) {
	seq := Delete(123, false)
	if !strings.Contains(seq, "a=d") || !strings.Contains(seq, "d=I") || !strings.Contains(seq, "i=123") {
		t.Fatalf("deletion: %q", seq)
	}
	if Delete(123, true) != ansi.TmuxPassthrough(seq) {
		t.Fatal("unwrapped deletion")
	}
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	for _, id := range []uint32{0, 0x1000000} {
		if Delete(id, false) != "" {
			t.Fatal("invalid deletion")
		}
		if _, _, err := Native(img, id, 1, 1, false); err == nil {
			t.Fatal("invalid ID accepted")
		}
	}
	if _, _, err := Native(img, 1, 121, 1, false); err == nil {
		t.Fatal("unbounded columns")
	}
}

func TestGIFUsesFirstFrame(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second := image.NewPaletted(first.Bounds(), palette)
	for i := range second.Pix {
		second.Pix[i] = 1
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := decoded.At(0, 0).RGBA()
	if r != 0 || g != 0 || b != 0 {
		t.Fatal("decoded a later frame")
	}
}

func TestBlocksCapsOutput(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	lines := strings.Split(ansi.Strip(Blocks(img, 1000, 1000, nil, colorprofile.TrueColor)), "\n")
	if len(lines) != 60 {
		t.Fatalf("rows=%d", len(lines))
	}
	for _, line := range lines {
		if ansi.StringWidth(line) != 120 {
			t.Fatalf("width=%d", ansi.StringWidth(line))
		}
	}
	if Blocks(nil, 1, 1, nil, colorprofile.TrueColor) != "" {
		t.Fatal("nil image rendered")
	}
}
