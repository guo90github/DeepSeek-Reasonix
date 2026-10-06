package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"reasonix/internal/tool"
)

// forgetTool deletes a saved memory the model judges wrong or stale. Like
// rememberTool it is stateful (bound to one project's Store), so boot constructs
// it and adds it to the registry.
type forgetTool struct{ store Store }

// NewForgetTool returns the `forget` tool bound to store.
func NewForgetTool(store Store) tool.Tool { return forgetTool{store: store} }

func (forgetTool) Name() string { return tool.HostForget }

func (forgetTool) Description() string {
	return "Delete a saved memory by name when it is wrong, stale, or superseded, so it stops loading into future sessions. " +
		"Use the stable project/<name>.md or global/<name>.md reference returned by memory search/read/list. " +
		"Prefer updating a memory with `remember` (reuse its name) over forget-then-recreate; reach for forget only when the fact should no longer exist at all."
}

func (forgetTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "Stable memory id, project/<name>.md or global/<name>.md reference, or legacy slug of the memory to archive."}
		},
		"required": ["name"]
	}`)
}

func (t forgetTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if in.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	memory, found := t.store.Read(in.Name)
	archive, err := t.store.Archive(in.Name)
	if err != nil {
		return "", err
	}
	if q, ok := QueueFromContext(ctx); ok && archive != "" {
		name := slug(strings.TrimSuffix(in.Name, ".md"))
		if found {
			name = memory.Name
		}
		q.QueueMemory("Forgot memory \"" + name + "\" — disregard its loaded guidance and background-index entry for the rest of this session.")
	}
	if archive != "" {
		if found {
			return fmt.Sprintf("Forgot memory %q (it no longer applies and will not load in future sessions; archived from %s).", in.Name, providerMemoryReference(memory)), nil
		}
		return fmt.Sprintf("Forgot memory %q (it no longer applies and will not load in future sessions; archived).", in.Name), nil
	}
	// Nothing was archived: a name that matched nothing used to read as a
	// successful delete, so a pruning pass could believe it dropped a fact it
	// never touched. Say so, and point at the real names.
	return fmt.Sprintf("No active memory named %q: nothing was archived and nothing changed.%s", in.Name, nearMissHint(t.store, in.Name)), nil
}

// nearMissHint names the closest active memories to a reference that matched
// nothing, so a mistyped prune is corrected in the same turn.
func nearMissHint(store Store, want string) string {
	names := nearMisses(store, want, 3)
	if len(names) == 0 {
		return ""
	}
	return " Closest active names: " + strings.Join(names, ", ") + "."
}

func nearMisses(store Store, want string, limit int) []string {
	target := slug(strings.TrimSuffix(strings.TrimSpace(want), ".md"))
	if target == "" {
		return nil
	}
	type scored struct {
		name string
		dist int
	}
	var hits []scored
	for _, fact := range store.ListAll() {
		name := slug(fact.Name)
		if name == "" || name == target {
			continue
		}
		dist := editDistance(target, name)
		if dist > 2 && !strings.Contains(name, target) && !strings.Contains(target, name) {
			continue
		}
		hits = append(hits, scored{name: fact.Name, dist: dist})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].dist != hits[j].dist {
			return hits[i].dist < hits[j].dist
		}
		return hits[i].name < hits[j].name
	})
	out := make([]string, 0, limit)
	seen := map[string]bool{}
	for _, hit := range hits {
		if seen[hit.name] {
			continue
		}
		seen[hit.name] = true
		out = append(out, hit.name)
		if len(out) == limit {
			break
		}
	}
	return out
}

// editDistance is the Levenshtein distance over runes.
func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, min(curr[j-1]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

func (forgetTool) ReadOnly() bool { return false }
