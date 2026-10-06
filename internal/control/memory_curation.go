package control

import (
	"fmt"
	"sort"
	"strings"

	"reasonix/internal/agent"
)

// Memory curation turns the session's recall records into the one action an agent can
// take — rewriting a fact that keeps being handed over and never shows up in a reply.
// The use proxy is weak evidence, so this reports a candidate, it never deletes one.

// recallCurationInjectedFloor is how many hand-overs make "never used" worth acting on.
const recallCurationInjectedFloor = 2

type recallCurationFact struct {
	id       string
	name     string
	injected int
	fetched  int
	used     int
	live     bool
}

// MemoryRecallTurns returns this session's recall records, oldest first.
func (c *Controller) MemoryRecallTurns() []agent.MemoryRecallTurn {
	path := strings.TrimSpace(c.SessionPath())
	if path == "" {
		return nil
	}
	meta, ok, err := agent.LoadBranchMeta(path)
	if err != nil || !ok {
		return nil
	}
	return meta.MemoryRecall
}

func renderMemoryCuration(api MemoryControl) string {
	turns := api.MemoryRecallTurns()
	if len(turns) == 0 {
		return "no recall recorded for this session yet"
	}
	facts := map[string]*recallCurationFact{}
	handed, fetched := 0, 0
	for _, turn := range turns {
		for _, hit := range turn.Hits {
			if hit.Injected == nil {
				continue
			}
			entry := facts[hit.ID]
			if entry == nil {
				entry = &recallCurationFact{id: hit.ID, name: hit.Name}
				facts[hit.ID] = entry
			}
			switch {
			case turn.Source == agent.MemoryRecallSourceTool:
				entry.fetched++
				fetched++
			case *hit.Injected:
				entry.injected++
				handed++
			}
			if hit.LikelyUsed != nil && *hit.LikelyUsed {
				entry.used++
			}
		}
	}
	if set := api.Memory(); set != nil {
		for _, fact := range set.Store.ListAll() {
			if entry := facts[fact.ID]; entry != nil {
				entry.live = true
			}
		}
	}
	rows := make([]*recallCurationFact, 0, len(facts))
	for _, entry := range facts {
		rows = append(rows, entry)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].injected != rows[j].injected {
			return rows[i].injected > rows[j].injected
		}
		return rows[i].name < rows[j].name
	})
	var b strings.Builder
	fmt.Fprintf(&b, "session recall: %d turn(s) · %d fact(s) touched · %d handed over · %d fetched on demand\n",
		len(turns), len(rows), handed, fetched)
	var stale []string
	for _, entry := range rows {
		label := entry.name
		if label == "" {
			label = entry.id
		}
		mark := ""
		if !entry.live {
			mark = " · no longer in the store"
		}
		fmt.Fprintf(&b, "  %s  injected %d · fetched %d · proxy-used %d%s\n",
			label, entry.injected, entry.fetched, entry.used, mark)
		if entry.injected >= recallCurationInjectedFloor && entry.used == 0 {
			stale = append(stale, label)
		}
	}
	if len(stale) > 0 {
		fmt.Fprintf(&b, "\ninjected repeatedly with no use signal — rewrite the keywords or delete:\n  %s\n",
			strings.Join(stale, ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}
