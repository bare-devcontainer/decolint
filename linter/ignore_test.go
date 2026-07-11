package linter

import (
	"testing"

	"github.com/tailscale/hujson"
)

func buildIndex(t *testing.T, src string) *ignoreIndex {
	t.Helper()
	root, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return buildIgnoreIndex(&root, newPositions([]byte(src)))
}

func TestIgnoreDirectives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		src    string
		line   int
		ruleID string
		want   bool
	}{
		{
			"no directive",
			`{
  "image": "ubuntu:latest"
}`,
			2, "no-image-latest", false,
		},
		{
			"next-line directive on preceding line",
			`{
  // decolint-ignore-next-line no-image-latest
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", true,
		},
		{
			"line directive on same line",
			`{
  "image": "ubuntu:latest" // decolint-ignore-line no-image-latest
}`,
			2, "no-image-latest", true,
		},
		{
			"next-line block comment on preceding line",
			`{
  /* decolint-ignore-next-line no-image-latest */
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", true,
		},
		{
			"line directive without rule ID suppresses all rules",
			`{
  "image": "ubuntu:latest" // decolint-ignore-line
}`,
			2, "no-image-latest", true,
		},
		{
			"next-line directive without rule ID suppresses all rules",
			`{
  // decolint-ignore-next-line
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", true,
		},
		{
			"unrelated rule ID is not suppressed",
			`{
  // decolint-ignore-next-line some-other-rule
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", false,
		},
		{
			"next-line directive two lines above does not apply",
			`{
  // decolint-ignore-next-line no-image-latest

  "image": "ubuntu:latest"
}`,
			4, "no-image-latest", false,
		},
		{
			"line directive on preceding line does not suppress next line",
			`{
  // decolint-ignore-line no-image-latest
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", false,
		},
		{
			"next-line directive on same line does not suppress that line",
			`{
  "image": "ubuntu:latest" // decolint-ignore-next-line no-image-latest
}`,
			2, "no-image-latest", false,
		},
		{
			"bare decolint-ignore is not a directive",
			`{
  // decolint-ignore no-image-latest
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", false,
		},
		{
			"file directive without rule IDs",
			`// decolint-ignore-file
{
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", true,
		},
		{
			"file directive with matching rule ID",
			`// decolint-ignore-file no-image-latest
{
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", true,
		},
		{
			"file directive with unrelated rule ID",
			`// decolint-ignore-file some-other-rule
{
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", false,
		},
		{
			"comma separated rule IDs",
			`{
  // decolint-ignore-next-line some-other-rule, no-image-latest
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", true,
		},
		{
			"similar prefix is not a directive",
			`{
  // decolint-ignore-lined no-image-latest
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", false,
		},
		{
			"directive in string value is not a directive",
			`{
  "name": "// decolint-ignore-file",
  "image": "ubuntu:latest"
}`,
			3, "no-image-latest", false,
		},
		{
			"comment before closing brace does not affect earlier lines",
			`{
  "image": "ubuntu:latest"
  // decolint-ignore-next-line no-image-latest
}`,
			2, "no-image-latest", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ix := buildIndex(t, tt.src)
			if got := ix.ignores(tt.line, tt.ruleID); got != tt.want {
				t.Errorf("ignores(%d, %q) = %v, want %v", tt.line, tt.ruleID, got, tt.want)
			}
		})
	}
}
