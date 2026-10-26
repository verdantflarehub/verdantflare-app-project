package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"github.com/verdantflarehub/verdantflare-app-project/internal/contentcache"
)

func (s *Service) mediaCache() *contentcache.Cache {
	s.cacheOnce.Do(func() {
		dir := os.Getenv("PROJECT_CONTENT_CACHE_DIR")
		if dir == "" {
			dir = thumbnailDir
		}
		budget := int64(2 << 30)
		if n, e := strconv.ParseInt(os.Getenv("PROJECT_CONTENT_CACHE_BYTES"), 10, 64); e == nil && n > 256<<20 {
			budget = n
		}
		s.cache = contentcache.New(filepath.Join(dir, "source"), budget*3/4)
		s.derived = contentcache.New(filepath.Join(dir, "derived"), budget/4)
	})
	return s.cache
}
func contentKey(p Principal, ref ContentRef) string {
	b, _ := json.Marshal([]string{p.OrganizationID, ref.StoreID, ref.ArtifactID, ref.VersionID})
	return string(b)
}

// Callers must establish membership and the file's fixed revision before using
// these byte caches. Current permission decisions are never cached here.
func (s *Service) cachedRead(ctx context.Context, p Principal, ref ContentRef, a Access, max int64) ([]byte, error) {
	return s.mediaCache().Get(ctx, "source:"+contentKey(p, ref), max, func() ([]byte, error) { return s.content.Read(ctx, p, ref, a, max) })
}
func (s *Service) cachedMetadata(ctx context.Context, p Principal, ref ContentRef, a Access) (ContentVersion, error) {
	b, e := s.mediaCache().Get(ctx, "metadata:"+contentKey(p, ref), 64<<10, func() ([]byte, error) {
		v, e := s.content.Metadata(ctx, p, ref, a)
		if e != nil {
			return nil, e
		}
		return json.Marshal(v)
	})
	var v ContentVersion
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil || v.ContentRef != ref || v.OrganizationID != p.OrganizationID || len(v.SHA256) != 64 || v.Size < 0 {
		return v, ErrNotReady
	}
	return v, nil
}
func checkedHash(b []byte, want string) bool {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]) == want
}

func (s *Service) derivedCache() *contentcache.Cache { s.mediaCache(); return s.derived }
