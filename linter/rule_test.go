package linter_test

import (
	"encoding/json/v2"
	"testing"

	"github.com/bare-devcontainer/decolint/linter"
)

func TestParsePlatform(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    linter.Platform
		wantErr bool
	}{
		{"vscode", "vscode", linter.PlatformVSCode, false},
		{"codespaces", "codespaces", linter.PlatformCodespaces, false},
		{"mixed case", "VSCode", linter.PlatformVSCode, false},
		{"unknown", "bogus", 0, true},
		{"empty", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := linter.ParsePlatform(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePlatform(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("ParsePlatform(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    linter.Category
		wantErr bool
	}{
		{"correctness", "correctness", linter.CategoryCorrectness, false},
		{"security", "security", linter.CategorySecurity, false},
		{"reproducibility", "reproducibility", linter.CategoryReproducibility, false},
		{"style", "style", linter.CategoryStyle, false},
		{"mixed case", "Security", linter.CategorySecurity, false},
		{"unknown", "bogus", 0, true},
		{"empty", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := linter.ParseCategory(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseCategory(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("ParseCategory(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCategoryString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		category linter.Category
		want     string
	}{
		{linter.CategoryCorrectness, "correctness"},
		{linter.CategorySecurity, "security"},
		{linter.CategoryReproducibility, "reproducibility"},
		{linter.CategoryStyle, "style"},
		{linter.Category(0), "unknown"}, // the zero value is deliberately not a valid category
	}
	for _, tt := range tests {
		if got := tt.category.String(); got != tt.want {
			t.Errorf("Category(%d).String() = %q, want %q", tt.category, got, tt.want)
		}
	}
}

func TestParseSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    linter.Severity
		wantErr bool
	}{
		{"off", "off", linter.SeverityOff, false},
		{"warn", "warn", linter.SeverityWarn, false},
		{"error", "error", linter.SeverityError, false},
		{"mixed case", "Error", linter.SeverityError, false},
		{"unknown", "bogus", 0, true},
		{"empty", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := linter.ParseSeverity(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSeverity(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("ParseSeverity(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSeverityJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		severity linter.Severity
		want     string
	}{
		{linter.SeverityOff, `"off"`},
		{linter.SeverityWarn, `"warn"`},
		{linter.SeverityError, `"error"`},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got, err := json.Marshal(tt.severity)
			if err != nil {
				t.Fatalf("Marshal(%v): %v", tt.severity, err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal(%v) = %s, want %s", tt.severity, got, tt.want)
			}

			var s linter.Severity
			if err := json.Unmarshal(got, &s); err != nil {
				t.Fatalf("Unmarshal(%s): %v", got, err)
			}
			if s != tt.severity {
				t.Errorf("Unmarshal(%s) = %v, want %v", got, s, tt.severity)
			}
		})
	}

	t.Run("invalid severity name", func(t *testing.T) {
		t.Parallel()
		var s linter.Severity
		if err := json.Unmarshal([]byte(`"critical"`), &s); err == nil {
			t.Error("Unmarshal(\"critical\"): got nil error, want an error")
		}
	})

	t.Run("non-string token", func(t *testing.T) {
		t.Parallel()
		var s linter.Severity
		if err := json.Unmarshal([]byte(`1`), &s); err == nil {
			t.Error("Unmarshal(1): got nil error, want an error")
		}
	})
}

func TestPlatformString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		platform linter.Platform
		want     string
	}{
		{linter.PlatformVSCode, "vscode"},
		{linter.PlatformCodespaces, "codespaces"},
	}
	for _, tt := range tests {
		if got := tt.platform.String(); got != tt.want {
			t.Errorf("Platform(%d).String() = %q, want %q", tt.platform, got, tt.want)
		}
	}
}
