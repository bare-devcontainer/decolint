package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestPinImageDigest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no image property", `{"name": "test"}`, nil},
		{"untagged image", `{"image": "ubuntu"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "pin-image-digest", Message: `image "ubuntu" is not pinned by digest; add an "@sha256:..." digest`},
		}},
		{"tagged image", `{"image": "ubuntu:24.04"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "pin-image-digest", Message: `image "ubuntu:24.04" is not pinned by digest; add an "@sha256:..." digest`},
		}},
		{"digest only", `{"image": "ubuntu@sha256:abc123"}`, nil},
		{"tag and digest", `{"image": "ubuntu:24.04@sha256:abc123"}`, nil},
		{"registry port without digest", `{"image": "localhost:5000/app:1.0"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "pin-image-digest", Message: `image "localhost:5000/app:1.0" is not pinned by digest; add an "@sha256:..." digest`},
		}},
		{"registry port with digest", `{"image": "localhost:5000/app@sha256:abc123"}`, nil},
		{"non-string image", `{"image": 42}`, nil},
		{"malformed digest missing hex", `{"image": "ubuntu@sha256"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "pin-image-digest", Message: `image "ubuntu@sha256" is not pinned by digest; add an "@sha256:..." digest`},
		}},
		{"malformed digest trailing garbage", `{"image": "ubuntu@sha256:abc123 "}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "pin-image-digest", Message: `image "ubuntu@sha256:abc123 " is not pinned by digest; add an "@sha256:..." digest`},
		}},
		{"at sign without digest", `{"image": "ubuntu@latest"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "pin-image-digest", Message: `image "ubuntu@latest" is not pinned by digest; add an "@sha256:..." digest`},
		}},
		{"position points at the value, not the key", `{
  // the container image
  "image": "ubuntu:24.04"
}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 3, Col: 12, RuleID: "pin-image-digest", Message: `image "ubuntu:24.04" is not pinned by digest; add an "@sha256:..." digest`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.PinImageDigest, linter.SeverityWarn, tt.src, tt.want)
		})
	}
}
