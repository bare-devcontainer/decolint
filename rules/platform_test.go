package rules

import (
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
)

func TestPlatformEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rulePlatforms []linter.Platform
		selected      []linter.Platform
		want          bool
	}{
		{
			"nil-platforms rule runs with no platforms selected",
			nil,
			nil,
			true,
		},
		{
			"nil-platforms rule runs regardless of selection",
			nil,
			[]linter.Platform{linter.PlatformVSCode},
			true,
		},
		{
			"vscode-tagged rule does not run with no platforms selected",
			[]linter.Platform{linter.PlatformVSCode},
			nil,
			false,
		},
		{
			"vscode-tagged rule runs when vscode is selected",
			[]linter.Platform{linter.PlatformVSCode},
			[]linter.Platform{linter.PlatformVSCode},
			true,
		},
		{
			"vscode-tagged rule does not run when codespaces is selected",
			[]linter.Platform{linter.PlatformVSCode},
			[]linter.Platform{linter.PlatformCodespaces},
			false,
		},
		{
			"rule tagged with multiple platforms runs if any is selected",
			[]linter.Platform{linter.PlatformVSCode, linter.PlatformCodespaces},
			[]linter.Platform{linter.PlatformCodespaces},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := platformEnabled(tt.rulePlatforms, tt.selected); got != tt.want {
				t.Errorf("platformEnabled(%v, %v) = %v, want %v", tt.rulePlatforms, tt.selected, got, tt.want)
			}
		})
	}
}
