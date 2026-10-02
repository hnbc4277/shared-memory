package attachment

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestMakeThumbnail_PNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	thumb, ok := MakeThumbnail("image/png", buf.Bytes())
	if !ok || len(thumb) == 0 {
		t.Fatalf("ok=%v len=%d", ok, len(thumb))
	}
	cfg, err := jpegConfig(thumb)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width > 256 || cfg.Height > 256 {
		t.Fatalf("thumb too large: %dx%d", cfg.Width, cfg.Height)
	}
}

func TestMakeThumbnail_SkipsNonImage(t *testing.T) {
	if _, ok := MakeThumbnail("application/pdf", []byte("%PDF")); ok {
		t.Fatal("expected skip")
	}
}

func jpegConfig(b []byte) (image.Config, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	return cfg, err
}
