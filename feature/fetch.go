package feature

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Size limits for downloaded content, so a misbehaving registry or tarball cannot exhaust memory.
const (
	// maxArchiveBytes caps a downloaded Feature archive (an OCI layer blob or a tarball).
	maxArchiveBytes = 64 << 20
	// maxMetadataBytes caps a devcontainer-feature.json, whether read from disk or from an archive.
	maxMetadataBytes = 4 << 20
)

// metadataFileName is the file declaring a Feature, located at the root of the Feature's directory
// or archive.
const metadataFileName = "devcontainer-feature.json"

// Fetcher retrieves Feature metadata for the references found in devcontainer.json files. It
// caches every result in memory for the lifetime of the Fetcher, including failures, so a
// reference shared by several files is fetched at most once per run.
type Fetcher struct {
	client *http.Client

	mu    sync.Mutex
	cache map[string]fetchResult
}

type fetchResult struct {
	md  *Metadata
	err error
}

// NewFetcher returns a Fetcher with a default HTTP client.
func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{Timeout: 30 * time.Second},
		cache:  map[string]fetchResult{},
	}
}

// Fetch retrieves the metadata of the Feature referenced by raw. baseDir is the directory
// containing the devcontainer.json that references the Feature; local references are resolved
// relative to it.
func (f *Fetcher) Fetch(ctx context.Context, raw string, baseDir string) (*Metadata, error) {
	ref, err := ParseRef(raw)
	if err != nil {
		return nil, err
	}

	key := raw
	if ref.Kind == KindLocal {
		// The same relative reference can name different directories depending on the referencing
		// file's location.
		key = "local:" + filepath.Join(baseDir, raw)
	}

	f.mu.Lock()
	res, ok := f.cache[key]
	f.mu.Unlock()
	if !ok {
		res.md, res.err = f.fetch(ctx, ref, baseDir)
		if res.err != nil {
			res.err = fmt.Errorf("fetch feature %q: %w", raw, res.err)
		}
		f.mu.Lock()
		f.cache[key] = res
		f.mu.Unlock()
	}
	return res.md, res.err
}

func (f *Fetcher) fetch(ctx context.Context, ref Ref, baseDir string) (*Metadata, error) {
	switch ref.Kind {
	case KindLocal:
		return fetchLocal(filepath.Join(baseDir, ref.Raw))
	case KindTarball:
		return f.fetchTarball(ctx, ref.Raw)
	default:
		return f.fetchOCI(ctx, ref)
	}
}

// fetchLocal reads the metadata of the Feature in directory dir.
func fetchLocal(dir string) (*Metadata, error) {
	path := filepath.Join(dir, metadataFileName)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxMetadataBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, maxMetadataBytes)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseMetadata(src)
}
