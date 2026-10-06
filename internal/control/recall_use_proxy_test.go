package control

import (
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/memory"
)

// The proxy must fire on the fact's own words and stay quiet when the only overlap is
// the ask itself — that overlap is the model echoing the question, not using the fact.
func TestJudgeRecallUseSeparatesTheFactsWordsFromTheAsk(t *testing.T) {
	yes, no := true, false
	turn := agent.MemoryRecallTurn{
		TurnSeq:      3,
		QueryExcerpt: "把支付部署到绿色集群",
		Hits: []agent.MemoryRecallTurnHit{
			{ID: "mem-green", Name: "green-cluster", Injected: &yes},
			{ID: "mem-db", Name: "nightly-db-backup", Injected: &yes},
			{ID: "mem-echo", Name: "ask-echo", Injected: &yes},
			{ID: "mem-dropped", Name: "green-cluster", Injected: &no},
		},
	}
	facts := map[string]memory.Memory{
		"mem-green":   {ID: "mem-green", Name: "green-cluster", Title: "Green cluster"},
		"mem-db":      {ID: "mem-db", Name: "nightly-db-backup", Keywords: "nightly backup"},
		"mem-echo":    {ID: "mem-echo", Name: "ask-echo", Keywords: "绿色集群"},
		"mem-dropped": {ID: "mem-dropped", Name: "green-cluster"},
	}
	reply := "已按 green cluster 放行，另外把支付部署到绿色集群的流程记下了。"
	got := judgeRecallUse(turn, facts, reply)

	if got.Hits[0].LikelyUsed == nil || !*got.Hits[0].LikelyUsed {
		t.Fatalf("hit0 = %+v, want the fact's own word to count as used", got.Hits[0])
	}
	if got.Hits[1].LikelyUsed == nil || *got.Hits[1].LikelyUsed {
		t.Fatalf("hit1 = %+v, want an unused fact left unused", got.Hits[1])
	}
	if got.Hits[2].LikelyUsed == nil || *got.Hits[2].LikelyUsed {
		t.Fatalf("hit2 = %+v, want the ask's own words subtracted", got.Hits[2])
	}
	if got.Hits[3].LikelyUsed != nil {
		t.Fatalf("hit3 = %+v, want a dropped fact left unjudged", got.Hits[3])
	}
}

// A fact whose words are all in the ask must not be credited from the echo alone.
func TestJudgeRecallUseIgnoresATermAlreadyInTheAsk(t *testing.T) {
	yes := true
	turn := agent.MemoryRecallTurn{
		TurnSeq:      1,
		QueryExcerpt: "green cluster rollout",
		Hits:         []agent.MemoryRecallTurnHit{{ID: "mem-a", Name: "green-cluster", Injected: &yes}},
	}
	facts := map[string]memory.Memory{"mem-a": {ID: "mem-a", Name: "green-cluster"}}
	got := judgeRecallUse(turn, facts, "green cluster rollout done")
	if got.Hits[0].LikelyUsed == nil || *got.Hits[0].LikelyUsed {
		t.Fatalf("hit = %+v, want nothing credited from the ask's own words", got.Hits[0])
	}
}
