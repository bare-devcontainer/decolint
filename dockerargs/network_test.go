package dockerargs

import "testing"

func TestNetworkTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value, want string
	}{
		{`host`, "host"},
		{`devnet`, "devnet"},
		// Docker compares the network mode exactly, so a differently cased spelling is a network name.
		{`HOST`, "HOST"},
		{`name=host`, "host"},
		{`NAME=HOST`, "host"},
		{`alias=web,name=host`, "host"},
		{`alias=web, name = host `, "host"},
		{`name=devnet,name=host`, "host"},
		{`name=host,name=devnet`, "devnet"},
		{`alias=host`, ""},
		{`name=host,web`, ""},
		// The field-list reading is chosen by an unspaced "key=value" appearing somewhere, so a value
		// whose every "=" has space around it is a network name that happens to contain one.
		{`name = host`, "name = host"},
		{`name =host`, "name =host"},
		// The fields are a CSV record, so a field may be quoted and a record the reader rejects names
		// no network.
		{`"name=host"`, "host"},
		{`"name=host",alias=web`, "host"},
		{` name=host `, "host"},
		{`name=host,"alias=web`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			if got := NetworkTarget(tt.value); got != tt.want {
				t.Errorf("NetworkTarget(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
