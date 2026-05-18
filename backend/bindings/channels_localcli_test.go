package bindings

import (
	"testing"

	"one-api/common/config"
)

// TestListProviderTypes_ContainsLocalCLI guards the wiring between the
// channel type registry and the Wails frontend dropdown. If a future
// refactor moves the dropdown population into the frontend or rewrites
// ListProviderTypes, this test will catch a silent regression where the
// "LocalCLI Gateway" option disappears from the New-Channel form.
func TestListProviderTypes_ContainsLocalCLI(t *testing.T) {
	api := NewChannelsAPI()
	got := api.ListProviderTypes()

	for _, m := range got {
		if m.Type == config.ChannelTypeLocalCLI {
			if m.Name != "LocalCLI Gateway" {
				t.Fatalf("LocalCLI entry name = %q, want %q", m.Name, "LocalCLI Gateway")
			}
			return
		}
	}
	t.Fatalf("ListProviderTypes() missing entry for ChannelTypeLocalCLI (%d); got %d entries", config.ChannelTypeLocalCLI, len(got))
}
