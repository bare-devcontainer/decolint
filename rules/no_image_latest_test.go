package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestNoImageLatest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"no image property", `{"name": "test"}`, nil},
		{"untagged image", `{"image": "ubuntu"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "no-image-latest", Message: `image "ubuntu" has no explicit tag; pin a specific version`},
		}},
		{"latest tag", `{"image": "ubuntu:latest"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "no-image-latest", Message: `image "ubuntu:latest" uses the "latest" tag; pin a specific version`},
		}},
		{"pinned tag", `{"image": "ubuntu:24.04"}`, nil},
		{"digest", `{"image": "ubuntu@sha256:abc123"}`, nil},
		{"registry port without tag", `{"image": "localhost:5000/app"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "no-image-latest", Message: `image "localhost:5000/app" has no explicit tag; pin a specific version`},
		}},
		{"registry port with tag", `{"image": "localhost:5000/app:1.0"}`, nil},
		{"registry port with latest", `{"image": "localhost:5000/app:latest"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 11, RuleID: "no-image-latest", Message: `image "localhost:5000/app:latest" uses the "latest" tag; pin a specific version`},
		}},
		{"non-string image", `{"image": 42}`, nil},
		{"position points at the value, not the key", `{
  // the container image
  "image": "ubuntu:latest"
}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 3, Col: 12, RuleID: "no-image-latest", Message: `image "ubuntu:latest" uses the "latest" tag; pin a specific version`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.NoImageLatest{}, linter.Warn, tt.src, tt.want)
		})
	}
}
