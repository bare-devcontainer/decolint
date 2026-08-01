package dockerargs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseSecurityOpt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		s     string
		want  SecurityOpt
		wantO bool
	}{
		{"seccomp=unconfined", SecurityOpt{Key: "seccomp", Value: "unconfined"}, true},
		{"seccomp=builtin", SecurityOpt{Key: "seccomp", Value: "builtin"}, true},
		{"seccomp=./profile.json", SecurityOpt{Key: "seccomp", Value: "./profile.json"}, true},
		{"apparmor=unconfined", SecurityOpt{Key: "apparmor", Value: "unconfined"}, true},

		// A ":" separates the key from the value only in an entry holding no "=".
		{"seccomp:unconfined", SecurityOpt{Key: "seccomp", Value: "unconfined"}, true},
		{"seccomp:builtin", SecurityOpt{Key: "seccomp", Value: "builtin"}, true},
		{"label=user:USER", SecurityOpt{Key: "label", Value: "user:USER"}, true},

		// The key is matched case-sensitively, so it is kept as written.
		{"SECCOMP=unconfined", SecurityOpt{Key: "SECCOMP", Value: "unconfined"}, true},
		{"seccomp=BUILTIN", SecurityOpt{Key: "seccomp", Value: "BUILTIN"}, true},

		{"no-new-privileges", SecurityOpt{Key: "no-new-privileges", Value: "true"}, true},
		{"no-new-privileges=true", SecurityOpt{Key: "no-new-privileges", Value: "true"}, true},
		{"no-new-privileges:true", SecurityOpt{Key: "no-new-privileges", Value: "true"}, true},
		{"no-new-privileges=1", SecurityOpt{Key: "no-new-privileges", Value: "1"}, true},
		{"no-new-privileges=false", SecurityOpt{Key: "no-new-privileges", Value: "false"}, true},
		{"no-new-privileges=", SecurityOpt{Key: "no-new-privileges"}, true},
		{"no-new-privileges:", SecurityOpt{Key: "no-new-privileges"}, true},
		{"NO-NEW-PRIVILEGES", SecurityOpt{}, false},

		// Every other option needs a value.
		{"seccomp", SecurityOpt{}, false},
		{"seccomp=", SecurityOpt{}, false},
		{"seccomp:", SecurityOpt{}, false},
		{"", SecurityOpt{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			t.Parallel()
			got, gotOK := ParseSecurityOpt(tt.s)
			if diff := cmp.Diff(tt.want, got); diff != "" || gotOK != tt.wantO {
				t.Errorf("ParseSecurityOpt(%q) ok = %v, want %v; mismatch (-want +got):\n%s", tt.s, gotOK, tt.wantO, diff)
			}
		})
	}
}
