package capability

import (
	"testing"

	"reasonix/internal/tool"
)

// I4: a tool that returns an image says so in the catalogue, so a session can find the channel
// without a person telling it. The name is the interface the runtime probed, not a promise some
// description happens to make (2026-10-05).
func TestACatalogueEntryCarriesTheToolCapabilities(t *testing.T) {
	entries := ToolEntries([]tool.ContractEntry{
		{Name: "view_image", ReadOnly: true, Capabilities: []string{"ImageTool"}},
		{Name: "bash", Capabilities: []string{"SnipHinter"}},
	})
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want one per contract entry", len(entries))
	}
	if got := entries[0].Capabilities; len(got) != 1 || got[0] != "ImageTool" {
		t.Fatalf("view_image capabilities = %v, want the image channel named", got)
	}
	if entries[0].ID != "tool:view_image" || !entries[0].ReadOnly {
		t.Fatalf("entry = %+v, want the tool id and read-only flag kept", entries[0])
	}
}
