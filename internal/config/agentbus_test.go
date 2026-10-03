package config

import (
	"os"
	"path/filepath"
	"testing"
)

// The [agentbus] section is an operator's only way to bound a board: every key has to land on
// its field, because a key that parses into nothing is a bound nobody set (AGENT_BUS §13.7, and
// §11.5.7 for the deliberation window silence is counted against).
func TestAgentBusSectionLandsOnItsFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REASONIX_HOME", home)
	body := `
[agentbus]
budget_board = 400
budget_subtree = 120
budget_node = 20
budget_turn = 5
dispatch_slots = 2
hearing_round_ttl_minutes = 30
hearing_max_rounds = 3
hearing_cooldown_minutes = 10
hearing_escalation_quota = 1
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadForRootReadOnly(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := AgentBusConfig{
		BudgetBoard: 400, BudgetSubtree: 120, BudgetNode: 20, BudgetTurn: 5,
		DispatchSlots: 2, HearingRoundTTLMinutes: 30, HearingMaxRounds: 3,
		HearingCooldownMinutes: 10, HearingEscalationQuota: 1,
	}
	if got := cfg.AgentBus; got != want {
		t.Fatalf("AgentBus = %+v, want %+v", got, want)
	}
}

// An absent section is an unconfigured host, not a set of invented bounds.
func TestAnAbsentAgentBusSectionLeavesEveryBoundOff(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfg, err := LoadForRootReadOnly(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentBus != (AgentBusConfig{}) {
		t.Fatalf("AgentBus = %+v, want every bound off", cfg.AgentBus)
	}
}
