package dockerargs

import "testing"

func TestCapability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{"SYS_PTRACE", "CAP_SYS_PTRACE"},
		{"sys_ptrace", "CAP_SYS_PTRACE"},
		{"CAP_SYS_PTRACE", "CAP_SYS_PTRACE"},
		{"cap_sys_ptrace", "CAP_SYS_PTRACE"},
		{"", "CAP_"},

		{"ALL", "ALL"},
		{"all", "ALL"},
		{"All", "ALL"},
		// "ALL" is a name of its own, so prefixing it names an ordinary — and unknown — capability.
		{"CAP_ALL", "CAP_ALL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Capability(tt.name); got != tt.want {
				t.Errorf("Capability(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
