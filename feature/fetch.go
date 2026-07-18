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
	maxArchiveBytes = 64 << 20 // 64 MB
	// maxMetadataBytes caps a devcontainer-feature.json, whether read from disk or from an archive.
	maxMetadataBytes = 4 << 20 // 4 MB
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

// Fetch retrieves the metadata of the Feature referenced by raw. dir and fileDir together locate
// the devcontainer.json that references the Feature (see linter.Context.Dir and
// linter.Context.FileDir): a local reference is resolved by joining fileDir with it and reading the
// result through dir, so the resolution cannot escape dir's boundary. dir and fileDir are unused
// for an OCI or tarball reference.
func (f *Fetcher) Fetch(ctx context.Context, raw string, dir *os.Root, fileDir string) (*Metadata, error) {
	ref, err := ParseRef(raw)
	if err != nil {
		return nil, err
	}

	key := raw
	if ref.Kind == KindLocal {
		// The same relative reference can name different directories depending on the referencing
		// file's location and the root it is confined to.
		key = fmt.Sprintf("local:%p:%s", dir, filepath.Join(fileDir, raw))
	}

	f.mu.Lock()
	res, ok := f.cache[key]
	f.mu.Unlock()
	if !ok {
		res.md, res.err = f.fetch(ctx, ref, dir, fileDir)
		if res.err != nil {
			res.err = fmt.Errorf("fetch feature %q: %w", raw, res.err)
		}
		f.mu.Lock()
		f.cache[key] = res
		f.mu.Unlock()
	}
	return res.md, res.err
}

func (f *Fetcher) fetch(ctx context.Context, ref Ref, dir *os.Root, fileDir string) (*Metadata, error) {
	switch ref.Kind {
	case KindLocal:
		return fetchLocal(dir, filepath.Join(fileDir, ref.Raw))
	case KindTarball:
		return f.fetchTarball(ctx, ref.Raw)
	default:
		return f.fetchOCI(ctx, ref)
	}
}

// fetchLocal reads the metadata of the Feature at featureDir, read through dir so its resolution
// cannot escape dir's boundary.
func fetchLocal(dir *os.Root, featureDir string) (*Metadata, error) {
	path := filepath.Join(featureDir, metadataFileName)
	info, err := dir.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() > maxMetadataBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, maxMetadataBytes)
	}
	src, err := dir.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseMetadata(src)
}
