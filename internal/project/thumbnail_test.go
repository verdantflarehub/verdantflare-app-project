package project

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"sync"
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
	// Small image should not be upscaled when requesting 640 edge.
	small := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	var smallRaw bytes.Buffer
	if err := png.Encode(&smallRaw, small); err != nil {
		t.Fatal(err)
	}
	smallEncoded, err := encodeThumbnail(smallRaw.Bytes(), 640)
	if err != nil {
		t.Fatal(err)
	}
	smallImg, _, err := image.Decode(bytes.NewReader(smallEncoded))
	if err != nil {
		t.Fatal(err)
	}
	if smallImg.Bounds().Dx() != 200 || smallImg.Bounds().Dy() != 100 {
		t.Fatalf("small image upscaled: %v", smallImg.Bounds())
	}
	if _, err = encodeThumbnail([]byte("not-an-image"), 320); err != ErrUnsupported {
		t.Fatalf("non-image error = %v", err)
	}
	// Oversized header (>40M pixels) rejected before full decode.
	if _, err = encodeThumbnail(oversizedPNGHeader(8000, 6000), 320); err != ErrTooLarge {
		t.Fatalf("oversized image error = %v, want ErrTooLarge", err)
	}
}

func TestEncodeThumbnailAntiAliasesHighFrequencyPattern(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1672, 941))
	for y := 0; y < 941; y++ {
		for x := 0; x < 1672; x++ {
			v := uint8(20)
			if ((x/2)+(y/2))%2 == 1 {
				v = 235
			}
			src.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, src); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		edge, wantW, wantH int
	}{
		{edge: 320, wantW: 320, wantH: 180},
		{edge: 640, wantW: 640, wantH: 360},
	} {
		encoded, err := encodeThumbnail(raw.Bytes(), tc.edge)
		if err != nil {
			t.Fatalf("edge=%d encode error: %v", tc.edge, err)
		}
		img, _, err := image.Decode(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("edge=%d decode error: %v", tc.edge, err)
		}
		if img.Bounds().Dx() != tc.wantW || img.Bounds().Dy() != tc.wantH {
			t.Fatalf("edge=%d bounds=%v, want %dx%d", tc.edge, img.Bounds(), tc.wantW, tc.wantH)
		}
		var totalDiff float64
		var samples int
		for y := 4; y < tc.wantH-4; y++ {
			for x := 4; x < tc.wantW-5; x++ {
				r1, _, _, _ := img.At(x, y).RGBA()
				r2, _, _, _ := img.At(x+1, y).RGBA()
				d := float64(r1>>8) - float64(r2>>8)
				if d < 0 {
					d = -d
				}
				totalDiff += d
				samples++
			}
		}
		meanDX := totalDiff / float64(samples)
		if meanDX > 15 {
			t.Fatalf("edge=%d high-frequency pattern aliased: mean dx=%.2f (want <= 15)", tc.edge, meanDX)
		}
	}
}

func oversizedPNGHeader(w, h uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte("\x89PNG\r\n\x1a\n"))
	var ihdr bytes.Buffer
	_ = binary.Write(&ihdr, binary.BigEndian, w)
	_ = binary.Write(&ihdr, binary.BigEndian, h)
	ihdr.Write([]byte{8, 6, 0, 0, 0})
	_ = binary.Write(&buf, binary.BigEndian, uint32(ihdr.Len()))
	buf.WriteString("IHDR")
	buf.Write(ihdr.Bytes())
	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte("IHDR"))
	_, _ = crc.Write(ihdr.Bytes())
	_ = binary.Write(&buf, binary.BigEndian, crc.Sum32())
	return buf.Bytes()
}

func TestThumbnailMIMEConcurrencyAndRateLimits(t *testing.T) {
	for _, valid := range []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/png; charset=binary"} {
		if !supportedImageMIME(valid) {
			t.Fatalf("expected %s to be supported", valid)
		}
	}
	for _, invalid := range []string{"video/mp4", "audio/mpeg", "text/markdown", "application/pdf"} {
		if supportedImageMIME(invalid) {
			t.Fatalf("expected %s to be rejected", invalid)
		}
	}

	subject := "test-org/test-user-rate"
	thumbMu.Lock()
	delete(thumbHits, subject)
	thumbMu.Unlock()
	for i := 0; i < 60; i++ {
		if !allowThumbnail(subject) {
			t.Fatalf("request %d rejected early", i)
		}
	}
	if allowThumbnail(subject) {
		t.Fatal("61st request within 10s should be rate limited")
	}

	// 2 active slots + 8 waiting slots allowed; 11th concurrent request rejected with ErrRateLimited.
	ctx := context.Background()
	if err := acquireThumbnailSlot(ctx); err != nil {
		t.Fatal(err)
	}
	if err := acquireThumbnailSlot(ctx); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			if err := acquireThumbnailSlot(ctx); err == nil {
				releaseThumbnailSlot()
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-ready
	}
	// Spin briefly until all 8 goroutines enter thumbWait.
	for {
		thumbMu.Lock()
		w := thumbWait
		thumbMu.Unlock()
		if w == 8 {
			break
		}
	}
	if err := acquireThumbnailSlot(ctx); err != ErrRateLimited {
		t.Fatalf("11th concurrent slot err = %v, want ErrRateLimited", err)
	}
	releaseThumbnailSlot()
	releaseThumbnailSlot()
	wg.Wait()

	rec := httptest.NewRecorder()
	httpError(rec, Principal{RequestID: id(1)}, ErrRateLimited)
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("status=%d retry-after=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
}
