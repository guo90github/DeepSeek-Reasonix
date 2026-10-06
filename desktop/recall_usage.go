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
	Available bool               `json:"available"`
	Sessions  int                `json:"sessions,omitempty"`
	Facts     []RecallUsageFact  `json:"facts,omitempty"`
	Queries   []RecallQueryUsage `json:"queries,omitempty"`
	Truncated bool               `json:"truncated,omitempty"`
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
	// Used counts the hand-overs the use proxy read as used; it never feeds a decision.
	Used int `json:"used,omitempty"`
}

// RecallQueryUsage is one query fingerprint's repeat pattern: the same question
// asked again and again. A query that keeps recalling and never injects points at
// the fact's own words not matching how the question is asked — recall is lexical.
type RecallQueryUsage struct {
	Hash        string   `json:"hash"`
	Turns       int      `json:"turns"`
	Sessions    int      `json:"sessions"`
	Injected    int      `json:"injected"`
	Dropped     int      `json:"dropped"`
	Unrecorded  int      `json:"unrecorded,omitempty"`
	LastTurnSeq int      `json:"lastTurnSeq,omitempty"`
	LastSession string   `json:"lastSession,omitempty"`
	Facts       []string `json:"facts,omitempty"`
}

// recallHitOutcome is a recorded decision. The record's own tri-state matters here:
// a hit written before the injected field existed is neither injected nor dropped.
type recallHitOutcome int

const (
	outcomeInjected recallHitOutcome = iota
	outcomeDropped
	outcomeUnrecorded
)

func recallOutcomeOf(injected *bool) recallHitOutcome {
	switch {
	case injected != nil && *injected:
		return outcomeInjected
	case injected != nil:
		return outcomeDropped
	default:
		return outcomeUnrecorded
	}
}

// recallUsageFactLimit caps the returned facts, recallUsageSessionLimit bounds the
// scan, and recallUsageRepeatTurns is how often one query must recur to be worth
// reporting: a single use is not a pattern.
const (
	recallUsageFactLimit    = 50
	recallUsageSessionLimit = 200
	recallUsageRepeatTurns  = 2
	recallUsageQueryLimit   = 25
	recallUsageQueryFacts   = 5
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
	queries := newRecallQueryFold()
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
			query := queries.observe(session.Path, turn)
			for _, hit := range turn.Hits {
				outcome := recallOutcomeOf(hit.Injected)
				fact := usage[hit.ID]
				if fact == nil {
					fact = &RecallUsageFact{
						ID: hit.ID, Name: hit.Name, Description: hit.Description,
						LastTurnSeq: turn.TurnSeq, LastSession: session.Path,
					}
					usage[hit.ID] = fact
				} else if fact.LastSession == session.Path && turn.TurnSeq > fact.LastTurnSeq {
					// Within the newest session that used it, the latest turn wins.
					fact.LastTurnSeq = turn.TurnSeq
				}
				fact.Uses++
				switch outcome {
				case outcomeInjected:
					fact.Injected++
				case outcomeDropped:
					fact.Dropped++
				default:
					fact.Unrecorded++
				}
				if outcome == outcomeInjected && hit.LikelyUsed != nil && *hit.LikelyUsed {
					fact.Used++
				}
				if hit.Revision > fact.UsedRevision {
					fact.UsedRevision = hit.Revision
				}
				queries.attribute(query, turn.QueryHash, hit.ID, outcome)
			}
		}
	}
	if len(usage) == 0 && queries.empty() {
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
	repeats, capped := queries.repeats()
	if capped {
		truncated = true
	}
	return RecallUsage{Available: true, Sessions: scanned, Facts: facts, Queries: repeats, Truncated: truncated}
}

// recallQueryFold accumulates the per-query repeat pattern of a scan: one entry per
// query hash, plus the session and fact sets that make a repeat readable.
type recallQueryFold struct {
	queries  map[string]*RecallQueryUsage
	sessions map[string]map[string]bool
	facts    map[string]map[string]bool
}

func newRecallQueryFold() recallQueryFold {
	return recallQueryFold{
		queries:  map[string]*RecallQueryUsage{},
		sessions: map[string]map[string]bool{},
		facts:    map[string]map[string]bool{},
	}
}

func (f recallQueryFold) empty() bool { return len(f.queries) == 0 }

// observe records one turn of one session, returning that turn's query entry (nil
// when the record carries no hash) so the caller can attribute the turn's hits.
func (f recallQueryFold) observe(sessionPath string, turn agent.MemoryRecallTurn) *RecallQueryUsage {
	if turn.QueryHash == "" {
		return nil
	}
	query := f.queries[turn.QueryHash]
	if query == nil {
		query = &RecallQueryUsage{Hash: turn.QueryHash, LastTurnSeq: turn.TurnSeq, LastSession: sessionPath}
		f.queries[turn.QueryHash] = query
		f.sessions[turn.QueryHash] = map[string]bool{}
		f.facts[turn.QueryHash] = map[string]bool{}
	}
	query.Turns++
	f.sessions[turn.QueryHash][sessionPath] = true
	if query.LastSession == sessionPath && turn.TurnSeq > query.LastTurnSeq {
		query.LastTurnSeq = turn.TurnSeq
	}
	return query
}

// attribute counts one hit for its query and remembers which facts it keeps matching.
func (f recallQueryFold) attribute(query *RecallQueryUsage, hash, factID string, outcome recallHitOutcome) {
	if query == nil {
		return
	}
	switch outcome {
	case outcomeInjected:
		query.Injected++
	case outcomeDropped:
		query.Dropped++
	default:
		query.Unrecorded++
	}
	if len(query.Facts) < recallUsageQueryFacts && !f.facts[hash][factID] {
		f.facts[hash][factID] = true
		query.Facts = append(query.Facts, factID)
	}
}

// repeats returns the hashes asked often enough to be a pattern, most-asked first,
// and whether the cap dropped any.
func (f recallQueryFold) repeats() ([]RecallQueryUsage, bool) {
	repeats := make([]RecallQueryUsage, 0, len(f.queries))
	for hash, query := range f.queries {
		query.Sessions = len(f.sessions[hash])
		if query.Turns < recallUsageRepeatTurns {
			continue
		}
		repeats = append(repeats, *query)
	}
	sort.SliceStable(repeats, func(i, j int) bool {
		if repeats[i].Turns != repeats[j].Turns {
			return repeats[i].Turns > repeats[j].Turns
		}
		if repeats[i].LastTurnSeq != repeats[j].LastTurnSeq {
			return repeats[i].LastTurnSeq > repeats[j].LastTurnSeq
		}
		return repeats[i].Hash < repeats[j].Hash
	})
	if len(repeats) > recallUsageQueryLimit {
		return repeats[:recallUsageQueryLimit], true
	}
	return repeats, false
}
