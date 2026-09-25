package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// argumentProbeTool mirrors edit_file's schema: three required string properties
// plus one optional token, and no "additionalProperties": false. That is the
// shape that accepted the near-miss key in silence while reporting only the
// missing required property.
type argumentProbeTool struct{}

func (argumentProbeTool) Name() string        { return "probe" }
func (argumentProbeTool) Description() string { return "probe schema" }
func (argumentProbeTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"},"source_token":{"type":"string"}},"required":["path","old_string","new_string"]}`)
}
func (argumentProbeTool) ReadOnly() bool { return true }
func (argumentProbeTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "probed", nil
}

type rawSchemaTool struct {
	schema json.RawMessage
}

func (rawSchemaTool) Name() string        { return "raw" }
func (rawSchemaTool) Description() string { return "" }
func (rawSchemaTool) ReadOnly() bool      { return true }
func (s rawSchemaTool) Schema() json.RawMessage {
	return s.schema
}
func (rawSchemaTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "", nil
}

func TestUndeclaredArgumentHintNamesTheMisspelledArgument(t *testing.T) {
	got := undeclaredArgumentHint(argumentProbeTool{}, json.RawMessage(`{"path":"a.go","old_string":"x","new_text":"y"}`))
	for _, want := range []string{
		`"new_text" is not declared by probe`,
		`declared arguments: "new_string", "old_string", "path", "source_token"`,
		`Did you mean "new_string"?`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("hint %q missing %q", got, want)
		}
	}
}

func TestUndeclaredArgumentHintStaysQuietWithoutUndeclaredKeys(t *testing.T) {
	for name, args := range map[string]string{
		"all declared":  `{"path":"a.go","old_string":"x","new_string":"y","source_token":"t"}`,
		"undecodable":   `{"path":`,
		"not an object": `["path"]`,
		"empty":         `{}`,
	} {
		if got := undeclaredArgumentHint(argumentProbeTool{}, json.RawMessage(args)); got != "" {
			t.Fatalf("%s: hint = %q, want none", name, got)
		}
	}
}

// A permissive schema accepts extra properties by design, so naming one as
// undeclared would send the caller after the wrong cause.
func TestUndeclaredArgumentHintStaysQuietOnPermissiveSchemas(t *testing.T) {
	for name, schema := range map[string]string{
		"true":      `{"type":"object","properties":{"a":{"type":"string"}},"required":["a"],"additionalProperties":true}`,
		"subschema": `{"type":"object","properties":{"a":{"type":"string"}},"required":["a"],"additionalProperties":{"type":"string"}}`,
	} {
		tool := rawSchemaTool{schema: json.RawMessage(schema)}
		if got := undeclaredArgumentHint(tool, json.RawMessage(`{"a":"v","stray":1}`)); got != "" {
			t.Fatalf("%s: hint = %q, want none", name, got)
		}
	}
}

func TestUndeclaredArgumentHintRefusesUnreadableSchemas(t *testing.T) {
	for name, schema := range map[string]string{
		"reference":   `{"$ref":"#/definitions/x","type":"object"}`,
		"composition": `{"type":"object","properties":{"a":{"type":"string"}},"anyOf":[{"required":["a"]}]}`,
		"not object":  `{"type":"array","items":{"type":"string"}}`,
		"invalid":     `{`,
	} {
		tool := rawSchemaTool{schema: json.RawMessage(schema)}
		if got := undeclaredArgumentHint(tool, json.RawMessage(`{"nope":"v"}`)); got != "" {
			t.Fatalf("%s: hint = %q, want none", name, got)
		}
	}
}

func TestUndeclaredArgumentHintBoundsBothLists(t *testing.T) {
	properties := make([]string, 0, 20)
	for i := range 20 {
		properties = append(properties, fmt.Sprintf(`"declared_%02d":{"type":"string"}`, i))
	}
	schema := json.RawMessage(`{"type":"object","properties":{` + strings.Join(properties, ",") + `}}`)
	args := json.RawMessage(`{"declared_00":"v","stray_a":1,"stray_b":2,"stray_c":3,"stray_d":4,"stray_e":5}`)
	got := undeclaredArgumentHint(rawSchemaTool{schema: schema}, args)
	if !strings.Contains(got, "and 2 more") {
		t.Fatalf("undeclared list not bounded: %q", got)
	}
	if !strings.Contains(got, "and 8 more") {
		t.Fatalf("declared list not bounded: %q", got)
	}
}

func TestUndeclaredArgumentHintNeedsNoNearMissToFire(t *testing.T) {
	got := undeclaredArgumentHint(argumentProbeTool{}, json.RawMessage(`{"path":"a.go","old_string":"x","new_string":"y","zzz":1}`))
	if !strings.Contains(got, `"zzz" is not declared by probe`) {
		t.Fatalf("hint = %q", got)
	}
	if strings.Contains(got, "Did you mean") {
		t.Fatalf("unrelated name must not draw a suggestion: %q", got)
	}
}

// TestUndeclaredArgumentReachesToolResult asserts the hint at its real boundary:
// the tool result the model reads back after a call it must correct.
func TestUndeclaredArgumentReachesToolResult(t *testing.T) {
	misspelled := provider.ToolCall{ID: "bad", Name: "probe", Arguments: `{"path":"a.go","old_string":"x","new_text":"y"}`}
	corrected := provider.ToolCall{ID: "fixed", Name: "probe", Arguments: `{"path":"a.go","old_string":"x","new_string":"y"}`}
	p := testutil.NewMock("test",
		testutil.Turn{ToolCalls: []provider.ToolCall{misspelled}},
		testutil.Turn{ToolCalls: []provider.ToolCall{corrected}},
		testutil.Turn{Text: "done"})
	reg := tool.NewRegistry()
	reg.Add(argumentProbeTool{})
	a := New(p, reg, NewSession("system"), Options{ModelRef: t.Name()}, event.Discard)
	if err := a.Run(withNoClosedLoop(context.Background()), "edit the file"); err != nil {
		t.Fatal(err)
	}
	var guidance string
	for _, m := range a.Session().Snapshot() {
		if m.Role == provider.RoleTool && m.ToolCallID == "bad" {
			guidance = m.Content
		}
	}
	if !strings.Contains(guidance, `"new_text" is not declared by probe`) || !strings.Contains(guidance, `Did you mean "new_string"?`) {
		t.Fatalf("tool result guidance = %q", guidance)
	}
	if p.CallCount() != 3 {
		t.Fatalf("calls=%d, want the correction to stay inside the turn (no network retry)", p.CallCount())
	}
}
