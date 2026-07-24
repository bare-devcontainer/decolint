package schema

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseVariant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    Variant
		wantErr bool
	}{
		{"off", VariantOff, false},
		{"base", VariantBase, false},
		{"main", VariantMain, false},
		{"", VariantOff, true},
		{"Main", VariantOff, true},
		{"bogus", VariantOff, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := ParseVariant(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseVariant(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil {
				if got != tt.want {
					t.Errorf("ParseVariant(%q) = %v, want %v", tt.in, got, tt.want)
				}
				if got.String() != tt.in {
					t.Errorf("Variant.String() = %q, want %q", got.String(), tt.in)
				}
			}
		})
	}
}

// messages validates src and returns just the diagnostic messages, positioning every finding at
// offset 0 since these tests assert wording rather than location (see TestValidate_Position).
func messages(t *testing.T, v Variant, kind Kind, src string) []string {
	t.Helper()
	diags, err := Validate(v, kind, []byte(src), func([]string) int { return 0 })
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(diags) == 0 {
		return nil
	}
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	return msgs
}

func TestValidate_Devcontainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		variant Variant
		src     string
		want    []string
	}{
		{
			name:    "valid image container",
			variant: VariantMain,
			src:     `{"image": "ubuntu@sha256:abc", "forwardPorts": [3000]}`,
			want:    nil,
		},
		{
			name:    "valid compose container",
			variant: VariantMain,
			src:     `{"dockerComposeFile": "compose.yaml", "service": "app", "workspaceFolder": "/w"}`,
			want:    nil,
		},
		{
			name:    "misspelled property suggests the correct name",
			variant: VariantMain,
			src:     `{"image": "ubuntu", "forwardPort": [3000]}`,
			want:    []string{`unknown property "forwardPort"; did you mean "forwardPorts"?`},
		},
		{
			name:    "wrong value type",
			variant: VariantMain,
			src:     `{"image": "ubuntu", "forwardPorts": "3000"}`,
			want:    []string{`property "/forwardPorts" must be array, but is string`},
		},
		{
			name:    "unsupported enum value",
			variant: VariantMain,
			src:     `{"dockerComposeFile": "c.yaml", "service": "app", "workspaceFolder": "/w", "shutdownAction": "bogus"}`,
			want:    []string{`property "/shutdownAction" has an unsupported value`},
		},
		{
			name:    "compose container missing a required property",
			variant: VariantMain,
			src:     `{"dockerComposeFile": "c.yaml", "service": "app"}`,
			want:    []string{`missing required property "workspaceFolder"`},
		},
		{
			name:    "VS Code extension property allowed by main",
			variant: VariantMain,
			src:     `{"image": "ubuntu", "customizations": {"vscode": {"extensions": ["golang.go"]}}}`,
			want:    nil,
		},
		{
			name:    "deprecated top-level VS Code property allowed by main",
			variant: VariantMain,
			src:     `{"image": "ubuntu", "extensions": ["golang.go"]}`,
			want:    nil,
		},
		{
			name:    "deprecated top-level VS Code property rejected by base",
			variant: VariantBase,
			src:     `{"image": "ubuntu", "extensions": ["golang.go"]}`,
			want:    []string{`unknown property "extensions"`},
		},
		{
			name:    "variable placeholders in string values are accepted",
			variant: VariantMain,
			src:     `{"image": "ubuntu", "containerEnv": {"P": "${localEnv:PATH}"}, "workspaceFolder": "${localWorkspaceFolder}"}`,
			want:    nil,
		},
		{
			name:    "off disables validation",
			variant: VariantOff,
			src:     `{"forwardPort": [3000]}`,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := messages(t, tt.variant, KindDevcontainer, tt.src)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("messages mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidate_Feature(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "valid feature",
			src:  `{"id": "my-feature", "version": "1.0.0", "name": "My Feature"}`,
			want: nil,
		},
		{
			name: "unknown property",
			src:  `{"id": "my-feature", "version": "1.0.0", "instalsAfter": []}`,
			want: []string{`unknown property "instalsAfter"; did you mean "installsAfter"?`},
		},
		{
			name: "wrong value type",
			src:  `{"id": "my-feature", "version": 1}`,
			want: []string{`property "/version" must be string, but is number`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// The feature schema is variant-independent; base and main must agree.
			for _, v := range []Variant{VariantBase, VariantMain} {
				got := messages(t, v, KindFeature, tt.src)
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("variant %s messages mismatch (-want +got):\n%s", v, diff)
				}
			}
		})
	}
}

func TestValidate_Position(t *testing.T) {
	t.Parallel()

	// offsetFor records the instance location it is asked to resolve, so the test can assert that a
	// finding is anchored at the offending property rather than the document root.
	src := `{"image": "ubuntu", "forwardPort": [3000]}`
	var gotLoc []string
	diags, err := Validate(VariantMain, KindDevcontainer, []byte(src), func(loc []string) int {
		gotLoc = loc
		return 7
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}
	if diff := cmp.Diff([]string{"forwardPort"}, gotLoc); diff != "" {
		t.Errorf("instance location mismatch (-want +got):\n%s", diff)
	}
	if diags[0].Offset != 7 {
		t.Errorf("Offset = %d, want 7 (from offsetFor)", diags[0].Offset)
	}
}

func TestRevision(t *testing.T) {
	t.Parallel()

	rev := Revision()
	if rev == "unknown" {
		t.Fatal("Revision() = unknown; expected the embedded REVISIONS.json to be read")
	}
	for _, want := range []string{"spec@", "vscode@"} {
		if !contains(rev, want) {
			t.Errorf("Revision() = %q, want it to contain %q", rev, want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
