package dashboard

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"github.com/cosgroveb/guhd/internal/source"
)

func previewImage() (source.Attachment, []byte) {
	img := image.NewRGBA(image.Rect(0, 0, 240, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 240; x++ {
			c := color.RGBA{uint8(40 + y), uint8(80 + y), uint8(180 + y/2), 255}
			ridge := 75 - x/4
			if x > 120 {
				ridge = 15 + (x-120)/3
			}
			if y > ridge {
				c = color.RGBA{uint8(50 + y/2), uint8(75 + y/3), 65, 255}
			}
			if y > 95 {
				c = color.RGBA{180, 130, 70, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var data bytes.Buffer
	_ = png.Encode(&data, img) // Encoding this in-memory RGBA image cannot fail.
	return source.Attachment{ID: "preview-landscape", Name: "trail.png", MIMEType: "image/png", Size: data.Len()}, data.Bytes()
}
