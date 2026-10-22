package project

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestEncodeThumbnailShrinksLongEdge(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 800, 400))
	src.Set(0, 0, color.NRGBA{R: 20, G: 120, B: 90, A: 255})
	var raw bytes.Buffer
	if err := png.Encode(&raw, src); err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeThumbnail(raw.Bytes(), 320)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) >= raw.Len() {
		t.Fatalf("thumbnail %d is not smaller than source %d", len(encoded), raw.Len())
	}
	img, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 320 || img.Bounds().Dy() != 160 {
		t.Fatalf("size = %v", img.Bounds())
	}
	if _, err = encodeThumbnail([]byte("not-an-image"), 320); err != ErrUnsupported {
		t.Fatalf("non-image error = %v", err)
	}
}
