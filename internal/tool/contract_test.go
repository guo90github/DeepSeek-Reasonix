package tool_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"reasonix/internal/provider"
	"reasonix/internal/tool"
	_ "reasonix/internal/tool/builtin"
)

func TestBuiltinToolContractDocumentation(t *testing.T) {
	entries := tool.BuiltinContractEntries()
	if len(entries) == 0 {
		t.Fatal("no built-in tool contract entries")
	}
	doc, err := os.ReadFile("../../docs/TOOL_CONTRACT.md")
	if err != nil {
		t.Fatalf("read docs/TOOL_CONTRACT.md: %v", err)
	}
	text := string(doc)
	for _, e := range entries {
		if !strings.Contains(text, "| `"+e.Name+"` |") {
			t.Errorf("documentation missing table row for %s", e.Name)
		}
		if !strings.Contains(text, "| `"+e.Name+"` | "+boolString(e.ReadOnly)+" |") {
			t.Errorf("documentation missing read-only flag for %s", e.Name)
		}
		if strings.TrimSpace(e.Description) == "" {
			t.Errorf("%s has empty description", e.Name)
		}
		if !json.Valid(e.Schema) {
			t.Errorf("%s schema is invalid JSON: %s", e.Name, e.Schema)
		}
		if got := string(provider.CanonicalizeSchema(e.Schema)); got != string(e.Schema) {
			t.Errorf("%s schema is not canonical", e.Name)
		}
	}
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// acceptsDefaultSnip lists built-in tools that deliberately take the
// ReadOnly-tiered default snip geometry instead of implementing
// tool.SnipHinter. A tool belongs here only if its output has no special shape
// a generic head/tail split would garble — typically tools whose results are
// short (todo_write, complete_step) or already structured small (edit results).
// Membership is an explicit decision, not a fallback: a new or renamed built-in
// that lands in neither this set nor SnipHinter fails TestEveryBuiltinDeclaresSnipStance,
// which is the guard against a context-maintenance strategy silently desyncing
// from the tool surface.
var acceptsDefaultSnip = map[string]bool{
	"bash_output":   true, // streamed job output; tailing handled by the job, not the snip pass
	"code_index":    true,
	"complete_step": true,
	"compress":      true,
	"delete_range":  true,
	"delete_symbol": true,
	"edit_file":     true,
	"kill_shell":    true,
	"move_file":     true,
	"multi_edit":    true,
	"notebook_edit": true,
	"todo_write":    true,
	"update_goal":   true,
	"view_image":    true, // short metadata only; image bytes travel outside text snipping
	"wait":          true,
	"write_file":    true,
	"agent_bus":     true, // one op's receipt, or the bounded view it was rendered from
}

func TestEveryBuiltinDeclaresSnipStance(t *testing.T) {
	for _, b := range tool.Builtins() {
		name := b.Name()
		_, hints := b.(tool.SnipHinter)
		switch {
		case hints && acceptsDefaultSnip[name]:
			t.Errorf("%s both implements SnipHinter and is listed in acceptsDefaultSnip; remove it from the list", name)
		case !hints && !acceptsDefaultSnip[name]:
			t.Errorf("built-in %q declares no snip stance: implement tool.SnipHinter for a tailored geometry, or add it to acceptsDefaultSnip if the ReadOnly-tiered default is right (this guards against a renamed/new tool silently taking a generic default)", name)
		}
	}
}

// declaredCapabilities is what each built-in says it can do. The snip stance above already has to
// be declared rather than defaulted; this is that rule for the other optional interfaces, so a tool
// that gains (or loses) one fails here until its author writes it down (I5, 2026-10-05). No built-in
// declares an MCP* capability: those belong to the MCP adapter's own tools. Rows follow
// tool.CapabilityNames' probe order, which is the order the comparison reads.
var declaredCapabilities = map[string][]string{
	"agent_bus":     {},
	"bash":          {"SnipHinter"},
	"bash_output":   {"ContextualTool"},
	"code_index":    {},
	"complete_step": {"ContextualTool", "PlanModeClassifier"},
	"compress":      {"PlanModeClassifier"},
	"delete_range":  {"Previewer"},
	"delete_symbol": {"Previewer"},
	"edit_file":     {"Previewer"},
	"glob":          {"SnipHinter"},
	"grep":          {"SnipHinter"},
	"kill_shell":    {"ContextualTool"},
	"ls":            {"SnipHinter"},
	"move_file":     {},
	"multi_edit":    {"Previewer"},
	"notebook_edit": {"Previewer"},
	"read_file":     {"SnipHinter"},
	"todo_write":    {},
	"update_goal":   {"ContextualTool", "PlanModeClassifier"},
	"view_image":    {"ImageTool"}, // the only built-in with an image channel
	"wait":          {"ContextualTool"},
	"web_fetch":     {"SnipHinter"},
	"write_file":    {"Previewer"},
}

func TestEveryBuiltinDeclaresItsCapabilities(t *testing.T) {
	carried := map[string][]string{}
	for _, e := range tool.BuiltinContractEntries() {
		carried[e.Name] = e.Capabilities
	}
	for _, b := range tool.Builtins() {
		actual := tool.CapabilityNames(b)
		declared, ok := declaredCapabilities[b.Name()]
		if !ok {
			t.Errorf("%s declares no capability set: add %q: {%s} to declaredCapabilities", b.Name(), b.Name(), strings.Join(actual, ", "))
			continue
		}
		if strings.Join(actual, ",") != strings.Join(declared, ",") {
			t.Errorf("%s capabilities = %v, but the table declares %v", b.Name(), actual, declared)
		}
		// The contract entry carries the same answer: catalogues are built from it, so a capability
		// that only appeared in this test would still be invisible everywhere else.
		if strings.Join(carried[b.Name()], ",") != strings.Join(actual, ",") {
			t.Errorf("%s: contract entry carries %v, the tool answers %v", b.Name(), carried[b.Name()], actual)
		}
	}
}
