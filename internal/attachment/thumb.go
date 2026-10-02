package attachment

import (
	"bytes"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"

	"golang.org/x/image/draw"
)

const thumbMaxEdge = 256

// MakeThumbnail returns a JPEG thumbnail for supported raster images.
// Non-images and decode failures return (nil, false).
func MakeThumbnail(mimeType string, data []byte) ([]byte, bool) {
	if !strings.HasPrefix(mimeType, "image/") || mimeType == "image/webp" {
		return nil, false
	}
	src, err := decodeImage(mimeType, data)
	if err != nil || src == nil {
		return nil, false
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, false
	}
	scale := float64(thumbMaxEdge) / float64(max(w, h))
	if scale > 1 {
		scale = 1
	}
	nw, nh := int(float64(w)*scale+0.5), int(float64(h)*scale+0.5)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, false
	}
	return out.Bytes(), true
}

func decodeImage(mimeType string, data []byte) (image.Image, error) {
	r := bytes.NewReader(data)
	switch mimeType {
	case "image/png":
		return png.Decode(r)
	case "image/jpeg":
		return jpeg.Decode(r)
	case "image/gif":
		return gif.Decode(r)
	default:
		img, _, err := image.Decode(r)
		return img, err
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
