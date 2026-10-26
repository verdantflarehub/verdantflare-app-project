package project

import (
	"context"
	"strings"
	"testing"
)

type countingArtifact struct {
	Artifact
	reads, metadata int
}

func (a *countingArtifact) Read(context.Context, Principal, ContentRef, Access, int64) ([]byte, error) {
	a.reads++
	return []byte("fixed bytes"), nil
}
func (a *countingArtifact) Metadata(_ context.Context, p Principal, ref ContentRef, _ Access) (ContentVersion, error) {
	a.metadata++
	return ContentVersion{ContentRef: ref, OrganizationID: p.OrganizationID, SHA256: strings.Repeat("a", 64), Size: 11}, nil
}

func TestFixedSourceAndMetadataReuseAcrossServiceRestartAndSizes(t *testing.T) {
	t.Setenv("PROJECT_CONTENT_CACHE_DIR", t.TempDir())
	a := &countingArtifact{}
	p := Principal{OrganizationID: "org"}
	ref := ContentRef{StoreID: "store", ArtifactID: "artifact", VersionID: "version"}
	for _, s := range []*Service{{content: a}, {content: a}} {
		for _, limit := range []int64{32 << 20, 128 << 20} {
			b, err := s.cachedRead(context.Background(), p, ref, Access{}, limit)
			if err != nil || string(b) != "fixed bytes" {
				t.Fatal(err, string(b))
			}
			if _, err = s.cachedMetadata(context.Background(), p, ref, Access{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if a.reads != 1 || a.metadata != 1 {
		t.Fatal("fixed source reloaded", a.reads, a.metadata)
	}
	s := &Service{content: a}
	p.OrganizationID = "another-org"
	if _, err := s.cachedRead(context.Background(), p, ref, Access{}, 32<<20); err != nil {
		t.Fatal(err)
	}
	if a.reads != 2 {
		t.Fatal("organizations shared cache")
	}
}
