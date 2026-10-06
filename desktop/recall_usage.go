package main

import (
	"sort"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/memory"
)

// RecallUsage answers, across a workspace's sessions, which fact memory actually
// used, how often, and whether the revision it used is still the current one
// (docs/50 §2.2 全局级). It stays content-free apart from each fact's own label.
type RecallUsage struct {
	Available bool              `json:"available"`
	Sessions  int               `json:"sessions,omitempty"`
	Facts     []RecallUsageFact `json:"facts,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

// RecallUsageFact is one fact's usage across those sessions. Superseded means a
// later revision of the same fact exists now, which is the失效模式 the record
// exists to surface: an old conclusion quoted again.
type RecallUsageFact struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	Uses            int    `json:"uses"`
	Injected        int    `json:"injected"`
	Dropped         int    `json:"dropped"`
	Unrecorded      int    `json:"unrecorded,omitempty"`
	LastTurnSeq     int    `json:"lastTurnSeq,omitempty"`
	LastSession     string `json:"lastSession,omitempty"`
	UsedRevision    int    `json:"usedRevision,omitempty"`
	CurrentRevision int    `json:"currentRevision,omitempty"`
	Superseded      bool   `json:"superseded,omitempty"`
	Live            bool   `json:"live"`
}

// recallUsageFactLimit caps the returned facts, and recallUsageSessionLimit
// bounds the scan so opening the panel never walks an unbounded history.
const (
	recallUsageFactLimit    = 50
	recallUsageSessionLimit = 200
)

// RecallUsageForTab aggregates the recall records of one tab's workspace.
func (a *App) RecallUsageForTab(tabID string) RecallUsage {
	tab := a.tabByID(tabID)
	if tab == nil {
		if tabID != "" {
			return RecallUsage{}
		}
		tab = a.activeTab()
	}
	if tab == nil {
		return RecallUsage{}
	}
	sessions := a.ListSessionsForTab(tab.ID)
	if len(sessions) == 0 {
		return RecallUsage{}
	}
	var liveFacts []memory.Memory
	if ctrl := a.ctrlByTabID(tab.ID); ctrl != nil {
		if set := ctrl.Memory(); set != nil {
			liveFacts = set.Store.ListAll()
		}
	}
	return recallUsageForSessions(sessions, liveFacts)
}

// recallUsageForSessions folds sidecar records into per-fact usage. Sessions are
// newest-first, so the first record seen for a fact is its most recent use.
func recallUsageForSessions(sessions []SessionMeta, liveFacts []memory.Memory) RecallUsage {
	truncated := false
	if len(sessions) > recallUsageSessionLimit {
		sessions = sessions[:recallUsageSessionLimit]
		truncated = true
	}
	usage := map[string]*RecallUsageFact{}
	scanned := 0
	for _, session := range sessions {
		if strings.TrimSpace(session.Path) == "" {
			continue
		}
		meta, ok, err := agent.LoadBranchMeta(session.Path)
		if err != nil || !ok {
			continue
		}
		scanned++
		for _, turn := range meta.MemoryRecall {
			for _, hit := range turn.Hits {
				fact := usage[hit.ID]
				if fact == nil {
					fact = &RecallUsageFact{
						ID: hit.ID, Name: hit.Name, Description: hit.Description,
						LastTurnSeq: turn.TurnSeq, LastSession: session.Path,
					}
					usage[hit.ID] = fact
				}
				fact.Uses++
				switch {
				case hit.Injected != nil && *hit.Injected:
					fact.Injected++
				case hit.Injected != nil:
					fact.Dropped++
				default:
					fact.Unrecorded++
				}
				if hit.Revision > fact.UsedRevision {
					fact.UsedRevision = hit.Revision
				}
			}
		}
	}
	if len(usage) == 0 {
		return RecallUsage{}
	}
	live := make(map[string]memory.Memory, len(liveFacts))
	for _, fact := range liveFacts {
		live[fact.ID] = fact
	}
	facts := make([]RecallUsageFact, 0, len(usage))
	for _, fact := range usage {
		if current, ok := live[fact.ID]; ok {
			fact.Live = true
			fact.CurrentRevision = current.Revision
			fact.Superseded = fact.UsedRevision > 0 && current.Revision > fact.UsedRevision
			if current.Name != "" {
				fact.Name = current.Name
			}
			if current.Description != "" {
				fact.Description = current.Description
			}
		}
		facts = append(facts, *fact)
	}
	sort.SliceStable(facts, func(i, j int) bool {
		if facts[i].Uses != facts[j].Uses {
			return facts[i].Uses > facts[j].Uses
		}
		if facts[i].LastTurnSeq != facts[j].LastTurnSeq {
			return facts[i].LastTurnSeq > facts[j].LastTurnSeq
		}
		return facts[i].ID < facts[j].ID
	})
	if len(facts) > recallUsageFactLimit {
		facts = facts[:recallUsageFactLimit]
		truncated = true
	}
	return RecallUsage{Available: true, Sessions: scanned, Facts: facts, Truncated: truncated}
}
