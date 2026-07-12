package rules_test

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
	"github.com/bare-devcontainer/decolint/rules"
)

func TestMissingWorkspaceMountFolder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []linter.Issue
	}{
		{"image with both workspaceMount and workspaceFolder", `{"image": "ubuntu:22.04", "workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind", "workspaceFolder": "/workspace"}`, nil},
		{"image with neither", `{"image": "ubuntu:22.04"}`, nil},
		{"image with only workspaceMount", `{"image": "ubuntu:22.04", "workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "missing-workspace-mount-folder", Message: `devcontainer.json sets "workspaceMount" but is missing "workspaceFolder"`},
		}},
		{"image with only workspaceFolder", `{"image": "ubuntu:22.04", "workspaceFolder": "/workspace"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "missing-workspace-mount-folder", Message: `devcontainer.json sets "workspaceFolder" but is missing "workspaceMount"`},
		}},
		{"build with only workspaceMount", `{"build": {"dockerfile": "Dockerfile"}, "workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind"}`, []linter.Issue{
			{Path: "devcontainer.json", Line: 1, Col: 1, RuleID: "missing-workspace-mount-folder", Message: `devcontainer.json sets "workspaceMount" but is missing "workspaceFolder"`},
		}},
		{"build with both", `{"build": {"dockerfile": "Dockerfile"}, "workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind", "workspaceFolder": "/workspace"}`, nil},
		{"dockerComposeFile with only workspaceMount is not flagged", `{"dockerComposeFile": "docker-compose.yml", "service": "app", "workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind"}`, nil},
		{"neither image, build, nor dockerComposeFile", `{"workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind"}`, nil},
		{"empty object", `{}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIssues(t, rules.MissingWorkspaceMountFolder, linter.SeverityError, tt.src, tt.want)
		})
	}
}
