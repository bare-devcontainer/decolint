package feature

import (
	"fmt"
	"net/http"
	"time"
)

// requestKind identifies which access policy applies to an external HTTP request. Every request
// feature makes to an external host goes through externalClient.do, so the policy lives in one
// place instead of being decided ad hoc at each call site.
type requestKind int

const (
	// requestKindRegistry is a request to an OCI registry endpoint (manifest, blob, or token).
	requestKindRegistry requestKind = iota
	// requestKindTarball is a request to fetch a Feature tarball referenced by a direct HTTP(S) URI.
	requestKindTarball
)

// externalClient is the single HTTP client used for every request feature makes to an external
// host. By default it refuses any request whose URL is not HTTPS. A requestKindRegistry request is
// additionally allowed to use plain HTTP when allowInsecureRegistry is set (see
// WithInsecureRegistry); a requestKindTarball request always requires HTTPS regardless of that
// option, since a devcontainer.json author can point a tarball reference at any URI, and its
// integrity has no other guarantee than the transport.
type externalClient struct {
	http                  *http.Client
	allowInsecureRegistry bool
}

// newExternalClient returns an externalClient with a default timeout.
func newExternalClient(allowInsecureRegistry bool) *externalClient {
	return &externalClient{
		http:                  &http.Client{Timeout: 30 * time.Second},
		allowInsecureRegistry: allowInsecureRegistry,
	}
}

// do sends req, refusing it outright if its scheme is not permitted for kind.
func (c *externalClient) do(req *http.Request, kind requestKind) (*http.Response, error) {
	if req.URL.Scheme != "https" && (kind != requestKindRegistry || !c.allowInsecureRegistry) {
		return nil, fmt.Errorf("refusing insecure request to %s: only HTTPS is allowed", req.URL.Redacted())
	}
	return c.http.Do(req)
}
