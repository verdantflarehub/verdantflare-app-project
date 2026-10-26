package project

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/verdantflarehub/verdantflare-app-project/internal/contentcache"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime"
	"os"
	"os/exec"
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
	Preview    string `json:"preview,omitempty"`
}
type ThumbnailResult struct {
	MIME string `json:"mime"`
	Data string `json:"data"`
}

const (
	thumbnailMaxBytes int64 = 32 << 20
)

var (
	thumbnailDir = contentcache.DefaultDir("project")
	thumbMu      sync.Mutex
	thumbSem     = make(chan struct{}, 2)
	thumbWait    int
	thumbHits    = map[string][]time.Time{}
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
	if !p.Valid() || !ValidID(r.ProjectID) || !ValidID(r.RevisionID) || !ValidID(r.FileID) || (r.Edge != 320 && r.Edge != 640) || (r.Preview != "" && r.Preview != "poster") {
		return ThumbnailResult{}, ErrInvalid
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
	meta, err := s.cachedMetadata(ctx, p, file.Content, Access{ProjectID: r.ProjectID, RevisionID: r.RevisionID})
	if err != nil {
		return ThumbnailResult{}, err
	}
	poster := r.Preview == "poster"
	mt, _, _ := mime.ParseMediaType(meta.MIME)
	if (!poster && !supportedImageMIME(meta.MIME)) || (poster && mt != "video/mp4" && mt != "video/webm" && mt != "video/ogg" && mt != "video/quicktime") {
		return ThumbnailResult{}, ErrUnsupported
	}
	max := thumbnailMaxBytes
	if poster {
		max = 128 << 20
	}
	if meta.Size > max {
		return ThumbnailResult{}, ErrTooLarge
	}
	identity, _ := json.Marshal([]any{p.OrganizationID, meta.SHA256, r.Edge, r.Preview, "webp-v1"})
	encoded, err := s.derivedCache().Get(ctx, "derived:"+string(identity), 4<<20, func() ([]byte, error) {
		if !allowThumbnail(p.OrganizationID + "/" + p.SubjectID) {
			return nil, ErrRateLimited
		}
		if err := acquireThumbnailSlot(ctx); err != nil {
			return nil, err
		}
		defer releaseThumbnailSlot()
		raw, err := s.cachedRead(ctx, p, file.Content, Access{ProjectID: r.ProjectID, RevisionID: r.RevisionID}, max)
		if err != nil {
			return nil, err
		}
		if !checkedHash(raw, meta.SHA256) {
			return nil, ErrNotReady
		}
		if poster {
			return encodePoster(ctx, raw, r.Edge)
		}
		return encodeThumbnail(raw, r.Edge)
	})
	if err != nil {
		return ThumbnailResult{}, err
	}
	return ThumbnailResult{MIME: "image/webp", Data: base64.StdEncoding.EncodeToString(encoded)}, nil
}

// Seekable input supports ordinary MP4s with a trailing moov box. The source
// and resulting poster are cached; ffmpeg runs only for a cold representation.
func encodePoster(ctx context.Context, raw []byte, edge int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	input, err := os.CreateTemp("", "vf-poster-*")
	if err != nil {
		return nil, ErrDependency
	}
	name := input.Name()
	defer os.Remove(name)
	if _, err = input.Write(raw); err != nil {
		input.Close()
		return nil, ErrDependency
	}
	if input.Close() != nil {
		return nil, ErrDependency
	}
	executable := os.Getenv("PROJECT_FFMPEG")
	if executable == "" {
		executable = "ffmpeg"
	}
	cmd := exec.CommandContext(ctx, executable, "-nostdin", "-v", "error", "-threads", "1", "-ss", "0", "-i", name, "-frames:v", "1", "-vf", "scale="+strconv.Itoa(edge)+":"+strconv.Itoa(edge)+":force_original_aspect_ratio=decrease", "-f", "image2pipe", "-vcodec", "png", "pipe:1")
	output, err := cmd.Output()
	if err != nil {
		return nil, ErrUnsupported
	}
	return encodeThumbnail(output, edge)
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
