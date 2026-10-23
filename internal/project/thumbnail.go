package project

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HugoSmits86/nativewebp"
	"golang.org/x/image/webp"
)

type ThumbnailRequest struct {
	ProjectID  string `json:"project_id"`
	RevisionID string `json:"revision_id"`
	FileID     string `json:"file_id"`
	Edge       int    `json:"edge"`
}
type ThumbnailResult struct {
	MIME string `json:"mime"`
	Data string `json:"data"`
}

const (
	thumbnailMaxBytes      int64 = 32 << 20
	thumbnailMaxCacheBytes int   = 512 << 10
)

type thumbCall struct {
	done chan struct{}
	res  ThumbnailResult
	err  error
}

var (
	thumbnailDir  = os.TempDir()
	thumbMu       sync.Mutex
	thumbSem      = make(chan struct{}, 2)
	thumbWait     int
	thumbHits     = map[string][]time.Time{}
	thumbInflight = map[string]*thumbCall{}
)

func ThumbnailCacheDir(dir string) {
	if dir != "" {
		thumbnailDir = dir
	}
}

func supportedImageMIME(raw string) bool {
	if raw == "" {
		return true
	}
	mt, _, err := mime.ParseMediaType(raw)
	if err != nil {
		mt = strings.TrimSpace(strings.Split(raw, ";")[0])
	}
	switch strings.ToLower(mt) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func (s *Service) Thumbnail(ctx context.Context, p Principal, r ThumbnailRequest) (ThumbnailResult, error) {
	if !p.Valid() || !ValidID(r.ProjectID) || !ValidID(r.RevisionID) || !ValidID(r.FileID) || (r.Edge != 320 && r.Edge != 640) {
		return ThumbnailResult{}, ErrInvalid
	}
	if !allowThumbnail(p.OrganizationID + "/" + p.SubjectID) {
		return ThumbnailResult{}, ErrRateLimited
	}
	opened, err := s.Open(ctx, p, r.ProjectID, r.RevisionID)
	if err != nil {
		return ThumbnailResult{}, err
	}
	var file *File
	for i := range opened.Manifest.Files {
		if opened.Manifest.Files[i].ID == r.FileID {
			file = &opened.Manifest.Files[i]
			break
		}
	}
	if file == nil {
		return ThumbnailResult{}, ErrNotFound
	}
	meta, err := s.content.Metadata(ctx, p, file.Content, Access{ProjectID: r.ProjectID, RevisionID: r.RevisionID})
	if err != nil {
		return ThumbnailResult{}, err
	}
	if !supportedImageMIME(meta.MIME) {
		return ThumbnailResult{}, ErrUnsupported
	}
	if meta.Size > thumbnailMaxBytes {
		return ThumbnailResult{}, ErrTooLarge
	}
	key := filepath.Join(thumbnailDir, p.OrganizationID, meta.SHA256, strconv.Itoa(r.Edge)+".webp")
	if cached, err := os.ReadFile(key); err == nil && len(cached) > 0 {
		now := time.Now()
		_ = os.Chtimes(key, now, now)
		return ThumbnailResult{MIME: "image/webp", Data: base64.StdEncoding.EncodeToString(cached)}, nil
	}

	thumbMu.Lock()
	if call, ok := thumbInflight[key]; ok {
		thumbMu.Unlock()
		select {
		case <-call.done:
			return call.res, call.err
		case <-ctx.Done():
			return ThumbnailResult{}, ctx.Err()
		}
	}
	call := &thumbCall{done: make(chan struct{})}
	thumbInflight[key] = call
	thumbMu.Unlock()

	defer func() {
		thumbMu.Lock()
		delete(thumbInflight, key)
		close(call.done)
		thumbMu.Unlock()
	}()

	if err := acquireThumbnailSlot(ctx); err != nil {
		call.err = err
		return ThumbnailResult{}, err
	}
	defer releaseThumbnailSlot()

	raw, err := s.content.Read(ctx, p, file.Content, Access{ProjectID: r.ProjectID, RevisionID: r.RevisionID}, thumbnailMaxBytes)
	if err != nil {
		call.err = err
		return ThumbnailResult{}, err
	}
	if sha256.Sum256(raw) != mustSHA(meta.SHA256) {
		call.err = ErrNotReady
		return ThumbnailResult{}, ErrNotReady
	}
	encoded, err := encodeThumbnail(raw, r.Edge)
	if err != nil {
		call.err = err
		return ThumbnailResult{}, err
	}
	if len(encoded) <= thumbnailMaxCacheBytes {
		if err = os.MkdirAll(filepath.Dir(key), 0o700); err == nil {
			_ = os.WriteFile(key, encoded, 0o600)
		}
	}
	call.res = ThumbnailResult{MIME: "image/webp", Data: base64.StdEncoding.EncodeToString(encoded)}
	return call.res, nil
}

func encodeThumbnail(raw []byte, edge int) ([]byte, error) {
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(raw)); err == nil {
		if format != "png" && format != "jpeg" && format != "gif" && format != "webp" {
			return nil, ErrUnsupported
		}
		if int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
			return nil, ErrTooLarge
		}
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		if decoded, gifErr := gif.Decode(bytes.NewReader(raw)); gifErr == nil {
			img, format, err = decoded, "gif", nil
		}
	}
	if err != nil || (format != "png" && format != "jpeg" && format != "gif" && format != "webp") {
		return nil, ErrUnsupported
	}
	bounds := img.Bounds()
	pixels := int64(bounds.Dx()) * int64(bounds.Dy())
	if pixels > 40_000_000 {
		return nil, ErrTooLarge
	}
	img = scaleLongEdge(img, edge)
	var buf bytes.Buffer
	if err = nativewebp.Encode(&buf, img, nil); err != nil {
		return nil, ErrDependency
	}
	return buf.Bytes(), nil
}

func scaleLongEdge(src image.Image, edge int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	long := w
	if h > long {
		long = h
	}
	if long <= edge || long == 0 {
		edge = long
	}
	nw, nh := w*edge/long, h*edge/long
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	if nw == w && nh == h {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + y*h/nh
		for x := 0; x < nw; x++ {
			dst.Set(x, y, src.At(b.Min.X+x*w/nw, sy))
		}
	}
	return dst
}

func acquireThumbnailSlot(ctx context.Context) error {
	select {
	case thumbSem <- struct{}{}:
		return nil
	default:
	}
	thumbMu.Lock()
	if thumbWait >= 8 {
		thumbMu.Unlock()
		return ErrRateLimited
	}
	thumbWait++
	thumbMu.Unlock()
	defer func() {
		thumbMu.Lock()
		thumbWait--
		thumbMu.Unlock()
	}()
	select {
	case thumbSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseThumbnailSlot() {
	<-thumbSem
}

func allowThumbnail(key string) bool {
	thumbMu.Lock()
	defer thumbMu.Unlock()
	now := time.Now()
	hits := thumbHits[key][:0]
	for _, at := range thumbHits[key] {
		if now.Sub(at) < 10*time.Second {
			hits = append(hits, at)
		}
	}
	if len(hits) >= 60 {
		thumbHits[key] = hits
		return false
	}
	thumbHits[key] = append(hits, now)
	return true
}

func mustSHA(hex string) [32]byte {
	var out [32]byte
	for i := 0; i < 32 && i*2+1 < len(hex); i++ {
		value, _ := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		out[i] = byte(value)
	}
	return out
}

func init() {
	image.RegisterFormat("webp", "RIFF", webp.Decode, webp.DecodeConfig)
	// Keep draw imported so NRGBA conversion stays available to encoders.
	_ = draw.Src
	_ = jpeg.DefaultQuality
	_ = png.Encoder{}
}
