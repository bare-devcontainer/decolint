package feature

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
)

// fetchTarball retrieves a Feature distributed as a direct HTTP(S) URI to its archive.
func (f *Fetcher) fetchTarball(ctx context.Context, url string) (*Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("new request %s: %w", url, err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	// Reject oversized archives up front when the server advertises the size.
	// ContentLength is -1 for chunked responses and may be inaccurate, so the
	// LimitReader below remains the actual enforcement.
	if resp.ContentLength > maxArchiveBytes {
		return nil, fmt.Errorf("archive %s reports Content-Length %d, exceeds %d bytes", url, resp.ContentLength, maxArchiveBytes)
	}

	src, err := metadataFromArchive(io.LimitReader(resp.Body, maxArchiveBytes))
	if err != nil {
		return nil, fmt.Errorf("read archive %s: %w", url, err)
	}
	return parseMetadata(src)
}

// metadataFromArchive extracts the devcontainer-feature.json at the root of the tar archive read
// from r. Features are published as gzip-compressed .tgz tarballs, both when referenced directly
// and when pushed to an OCI registry (the .tgz is the layer blob). Plain tar is accepted
// defensively: the application/vnd.devcontainers.layer.v1+tar media type does not itself mandate a
// compression scheme, so the leading bytes are inspected to decide whether to decompress.
func metadataFromArchive(r io.Reader) ([]byte, error) {
	br := bufio.NewReader(r)
	if magic, err := br.Peek(2); err == nil && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("new gzip reader: %w", err)
		}
		defer func() { _ = gz.Close() }()
		return metadataFromTar(gz)
	}
	return metadataFromTar(br)
}

// metadataFromTar scans the tar stream read from r for the devcontainer-feature.json entry at the
// archive root and returns its content.
func metadataFromTar(r io.Reader) ([]byte, error) {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("no %s in archive", metadataFileName)
		}
		if err != nil {
			return nil, fmt.Errorf("read tar entry: %w", err)
		}
		if path.Clean(hdr.Name) != metadataFileName {
			continue
		}
		src, err := io.ReadAll(io.LimitReader(tr, maxMetadataBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", metadataFileName, err)
		}
		if len(src) > maxMetadataBytes {
			return nil, fmt.Errorf("%s exceeds %d bytes", metadataFileName, maxMetadataBytes)
		}
		return src, nil
	}
}
