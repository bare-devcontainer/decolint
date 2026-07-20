package feature

import (
	"archive/tar"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// tarFile is one entry written into a test tar archive.
type tarFile struct {
	name    string
	content []byte
}

// tarBytes builds an uncompressed tar archive containing files, in order.
func tarBytes(t *testing.T, files ...tarFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMetadataFromArchive_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		archive []byte
		wantErr string
	}{
		{
			name:    "no metadata entry",
			archive: tarBytes(t, tarFile{"./install.sh", []byte("#!/bin/sh\n")}),
			wantErr: "no " + metadataFileName,
		},
		{
			name:    "metadata exceeds size limit",
			archive: tarBytes(t, tarFile{"./" + metadataFileName, bytes.Repeat([]byte("a"), maxMetadataBytes+1)}),
			wantErr: "exceeds",
		},
		{
			// Bytes that are neither gzip (no 0x1f 0x8b magic) nor a valid tar header, so the tar
			// reader fails with something other than a clean EOF.
			name:    "corrupt tar stream",
			archive: []byte("this is not a valid tar archive"),
			wantErr: "read tar entry",
		},
		{
			// The gzip magic bytes followed by a byte that is not a valid compression method, so the
			// gzip reader rejects the header.
			name:    "malformed gzip header",
			archive: []byte{0x1f, 0x8b, 0xff, 0xff, 0xff, 0xff},
			wantErr: "gzip",
		},
		{
			// A complete metadata header (the leading 512-byte block) whose declared 2000-byte content
			// is cut short, so reading the entry fails partway through rather than at a clean boundary.
			name:    "truncated metadata content",
			archive: tarBytes(t, tarFile{"./" + metadataFileName, bytes.Repeat([]byte("a"), 2000)})[:512+500],
			wantErr: "read " + metadataFileName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := metadataFromArchive(bytes.NewReader(tt.archive))
			if err == nil {
				t.Fatalf("metadataFromArchive: got nil error, want one mentioning %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestFetchTarball_CorruptArchive(t *testing.T) {
	t.Parallel()

	// A 200 response whose body is neither gzip nor a valid tar, exercising the archive-read error
	// path of fetchTarball (as opposed to a transport or status failure).
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("garbage, not an archive"))
	}))
	defer srv.Close()

	f := NewFetcher()
	f.client = srv.Client()
	_, err := f.Fetch(t.Context(), srv.URL+"/feature.tgz", nil, "")
	if err == nil {
		t.Fatal("Fetch of a corrupt archive: got nil error")
	}
	if !strings.Contains(err.Error(), "read archive") {
		t.Errorf("error = %v, want it to mention reading the archive", err)
	}
}

func TestFetchTarball_ContentLengthTooLarge(t *testing.T) {
	t.Parallel()

	// Advertise a Content-Length beyond the archive cap without sending a body, so the size is
	// rejected up front before any archive bytes are read. The response is written raw over a
	// hijacked connection to decouple the declared length from the bytes actually sent.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Errorf("ResponseWriter does not support hijacking")
			return
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = fmt.Fprintf(bufrw, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", maxArchiveBytes+1)
		_ = bufrw.Flush()
	}))
	defer srv.Close()

	f := NewFetcher()
	f.client = srv.Client()
	_, err := f.Fetch(t.Context(), srv.URL+"/feature.tgz", nil, "")
	if err == nil {
		t.Fatal("Fetch of an archive with an oversized Content-Length: got nil error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want it to mention exceeding the size limit", err)
	}
}
