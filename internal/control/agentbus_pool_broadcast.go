package control

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// agentBusWakePoolMax caps how many pooled steps one wake names. The wake is a pointer, not an
// inventory: whoever wants the rest asks action=pool, and an uncapped list would put a whole board
// into one injected item (F53, 2026-10-05).
const agentBusWakePoolMax = 8

// withPoolNotice adds the common pool to this host's wakes and gives one to the participants
// nothing else would have woken.
//
// F53's finding is that the pool is the one part of the board every derivation ignores: its steps
// have no requester to wake and no assignee to address, so the session that could take them — the
// idle one — is exactly who never hears. The pool rides on top of the kernel's key rather than
// inside it, so a pool that changed wakes again while an unchanged one stays silent, and one
// generic wake per session stays the ceiling (F53/F40, 2026-10-05).
func withPoolNotice(targets []agentbus.WakeTarget, st *board.State, audience []string) []agentbus.WakeTarget {
	ids := pooledStepIDs(st)
	if len(ids) == 0 {
		return targets
	}
	suffix := ":pool-" + poolDigest(ids)
	at := make(map[string]int, len(targets))
	for i, t := range targets {
		at[t.Participant] = i
	}
	for _, participant := range audience {
		participant = strings.TrimSpace(participant)
		if participant == "" {
			continue
		}
		if i, held := at[participant]; held {
			targets[i].Pool = ids
			targets[i].Key += suffix
			continue
		}
		targets = append(targets, agentbus.WakeTarget{
			Participant: participant,
			Key:         "agentbus-wake:" + participant + suffix,
			Pool:        ids,
		})
	}
	return targets
}

// pooledStepIDs lists the steps nobody holds and nobody waits on, capped: the wake points at them,
// action=pool is where a reader gets the full list.
func pooledStepIDs(st *board.State) []string {
	pool := agentbus.BuildPool(st)
	if len(pool) == 0 {
		return nil
	}
	ids := make([]string, 0, min(len(pool), agentBusWakePoolMax))
	for _, entry := range pool {
		if len(ids) == agentBusWakePoolMax {
			break
		}
		ids = append(ids, entry.ID)
	}
	return ids
}

// poolDigest names one pool state, so the wake ledger treats a changed pool as new work and an
// unchanged one as already delivered.
func poolDigest(ids []string) string {
	sum := sha256.Sum256([]byte(strings.Join(ids, "\x00")))
	return hex.EncodeToString(sum[:4])
}

// agentBusWakePoolAudience is who hears about the pool: the sessions on this board's roster. A
// participant no session here speaks as is not a wake this host can deliver, and F49's roster is
// the only list of who is here (2026-10-05).
func (c *Controller) agentBusWakePoolAudience() []string {
	refs, err := c.AgentBusParticipants()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.Participant)
	}
	return out
}
