// The provider-visible memory index: a one-line-per-fact first layer. Each
// entry is a bounded label plus the fact's stable id as its handle; bodies and
// descriptions stay in the second layer, fetched on demand.
package memory

import (
	"fmt"
	"sort"
	"strings"
)

// Index returns the provider-visible index that loads into the cached
// prefix: every active fact from both scopes, shadowed global facts annotated
// rather than hidden — the index agrees with the project-over-global rule
// recall enforces (#7995). The per-directory MEMORY.md files keep their
// unqualified, descriptive format.
func (s Store) Index() string {
	memories := s.ListAll()
	if len(memories) == 0 {
		return ""
	}
	shadowed := map[string]string{} // global fact ID -> winning project reference
	for _, o := range FindOverrides(memories) {
		shadowed[o.Global.ID] = providerMemoryReference(o.Project)
	}
	sort.SliceStable(memories, func(i, j int) bool {
		if memories[i].Name != memories[j].Name {
			return memories[i].Name < memories[j].Name
		}
		return NormalizeFactScope(string(memories[i].Scope)) == FactScopeProject
	})
	var b strings.Builder
	seen := map[string]bool{} // collapse legacy migration duplicates (same qualified ref)
	for _, memory := range memories {
		if ref := providerMemoryReference(memory); seen[ref] {
			continue
		} else {
			seen[ref] = true
		}
		b.WriteString(renderQualifiedIndexLine(memory))
		if winner, ok := shadowed[memory.ID]; ok &&
			NormalizeFactScope(string(memory.Scope)) == FactScopeGlobal {
			b.WriteString(" (overridden by " + winner + ")")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// maxFirstLayerRunes bounds the index's free text per fact. The resident
// snapshot is a table of contents: one glance per fact, never its contents,
// and the glance must not grow with the fact's title length (docs/50 §十).
const maxFirstLayerRunes = 50

// renderQualifiedIndexLine is the provider-index variant of renderIndexLine.
// The link is the fact's stable id — a handle the memory tool resolves without
// a filesystem path — so a name collision across scopes can never be misread
// and a long kebab name never rides the resident snapshot.
func renderQualifiedIndexLine(m Memory) string {
	marker := ""
	if ResolveActivation(m) == ActivationPinned {
		marker = " pinned"
	}
	ref := m.ID
	if ref == "" {
		ref = providerMemoryReference(m)
	}
	label, summary := firstLayerText(m)
	line := fmt.Sprintf("- [%s](%s) — [%s/%s%s]",
		label, ref, NormalizeFactScope(string(m.Scope)), NormalizeType(string(m.Type)), marker)
	if summary != "" {
		line += " " + summary
	}
	return line
}

// firstLayerText splits the budget between the display label and its one-line
// description: the label takes what it needs, the summary what is left. A cut
// side keeps an ellipsis inside the budget rather than being dropped.
func firstLayerText(m Memory) (label, summary string) {
	label = oneLine(displayTitle(m.Title, m.Name))
	if len([]rune(label)) > maxFirstLayerRunes {
		return clampRunes(label, maxFirstLayerRunes), ""
	}
	return label, clampRunes(oneLine(m.Description), maxFirstLayerRunes-len([]rune(label)))
}

func clampRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit < 2 {
		return string(runes[:limit])
	}
	return strings.TrimRight(string(runes[:limit-1]), " ") + "…"
}
