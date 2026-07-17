package feature

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExternalClientDo(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	newReq := func(t *testing.T) *http.Request {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		return req
	}

	tests := []struct {
		name                  string
		kind                  requestKind
		allowInsecureRegistry bool
		wantErr               bool
	}{
		{"HTTP registry request rejected by default", requestKindRegistry, false, true},
		{"HTTP registry request allowed when opted in", requestKindRegistry, true, false},
		{"HTTP tarball request rejected by default", requestKindTarball, false, true},
		{"HTTP tarball request rejected even when the registry option is set", requestKindTarball, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newExternalClient(tt.allowInsecureRegistry)
			resp, err := c.do(newReq(t), tt.kind)
			if tt.wantErr {
				if err == nil {
					t.Fatal("do: got nil error, want a refusal")
				}
				if !strings.Contains(err.Error(), "HTTPS") {
					t.Errorf("err = %q, want it to mention HTTPS", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			_ = resp.Body.Close()
		})
	}
}

func TestExternalClientDoRedactsCredentialsInError(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodGet, "http://user:secret@example.com/blob", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := newExternalClient(false)
	_, err = c.do(req, requestKindRegistry)
	if err == nil {
		t.Fatal("do: got nil error, want a refusal")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("err = %q, want the password redacted", err)
	}
}
