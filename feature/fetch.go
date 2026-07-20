package feature

import (
	"context"
	"fmt"
	"io"
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
	// maxDecompressedBytes caps the bytes read out of a gzip stream, so a highly compressed
	// archive (a decompression bomb) cannot burn CPU expanding entries that precede the metadata.
	maxDecompressedBytes = 256 << 20 // 256 MB
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
	log    io.Writer

	mu    sync.Mutex
	cache map[string]fetchResult
}

type fetchResult struct {
	md  *Metadata
	err error
}

// Option configures a Fetcher created by NewFetcher.
type Option func(*Fetcher)

// WithLogWriter announces each remote download (an OCI artifact or a tarball) as a human-readable
// line on w. Without it a Fetcher downloads silently.
func WithLogWriter(w io.Writer) Option {
	return func(f *Fetcher) { f.log = w }
}

// NewFetcher returns a Fetcher with a default HTTP client, configured by the given options.
func NewFetcher(opts ...Option) *Fetcher {
	f := &Fetcher{
		client: &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: refuseInsecureRedirect,
		},
		log:   io.Discard,
		cache: map[string]fetchResult{},
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// refuseInsecureRedirect rejects a redirect that downgrades an HTTPS request to plain HTTP. A tarball
// or registry response is otherwise free to bounce a request to an internal host over plain HTTP,
// dropping the transport guarantee the original HTTPS reference relied on. A chain that started over
// plain HTTP (a loopback OCI registry) is left alone, since there is no downgrade to prevent.
func refuseInsecureRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing insecure redirect to %s", req.URL.Redacted())
	}
	return nil
}

// Fetch retrieves the metadata of the Feature referenced by raw. fsRoot and configDir together
// locate the devcontainer.json that references the Feature (fsRoot is discovery.ConfigFile.Root and
// configDir is the directory of its Path): fsRoot is the boundary every filesystem access is
// confined to, and configDir is the referencing file's directory within it. A local reference is
// resolved by joining configDir with it and reading the result through fsRoot, so the resolution
// cannot escape fsRoot's boundary. fsRoot and configDir are unused for an OCI or tarball reference.
func (f *Fetcher) Fetch(ctx context.Context, raw string, fsRoot *os.Root, configDir string) (*Metadata, error) {
	ref, err := ParseRef(raw)
	if err != nil {
		return nil, err
	}

	key := raw
	if ref.Kind == KindLocal {
		// The same relative reference can name different directories depending on the referencing
		// file's location and the root it is confined to.
		key = fmt.Sprintf("local:%p:%s", fsRoot, filepath.Join(configDir, raw))
	}

	f.mu.Lock()
	res, ok := f.cache[key]
	f.mu.Unlock()
	if !ok {
		res.md, res.err = f.fetch(ctx, ref, fsRoot, configDir)
		if res.err != nil {
			res.err = fmt.Errorf("fetch feature %q: %w", raw, res.err)
		}
		f.mu.Lock()
		f.cache[key] = res
		f.mu.Unlock()
	}
	return res.md, res.err
}

func (f *Fetcher) fetch(ctx context.Context, ref Ref, fsRoot *os.Root, configDir string) (*Metadata, error) {
	switch ref.Kind {
	case KindLocal:
		return fetchLocal(fsRoot, filepath.Join(configDir, ref.Raw))
	case KindTarball:
		_, _ = fmt.Fprintf(f.log, "Downloading feature(%s)\n", ref.Raw)
		return f.fetchTarball(ctx, ref.Raw)
	default:
		_, _ = fmt.Fprintf(f.log, "Downloading feature(%s)\n", ref.Raw)
		return f.fetchOCI(ctx, ref)
	}
}

// fetchLocal reads the metadata of the Feature at featureDir, read through fsRoot so its resolution
// cannot escape fsRoot's boundary.
func fetchLocal(fsRoot *os.Root, featureDir string) (*Metadata, error) {
	path := filepath.Join(featureDir, metadataFileName)
	info, err := fsRoot.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() > maxMetadataBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, maxMetadataBytes)
	}
	src, err := fsRoot.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseMetadata(src)
}
