package control

import (
	"context"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// refusalMemo is this state's memo of refused steps, created on the first refusal: a session that
// never sees one never allocates anything, and no other session's refusals can reach inside it.
// One memo for the whole process leaked a refusal into work handed out against another account
// (2026-10-05).
func (b *agentBusState) refusalMemo() *dispatchRefusalMemo {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dispatchRefusals == nil {
		b.dispatchRefusals = &dispatchRefusalMemo{}
	}
	return b.dispatchRefusals
}

// noteRefusedStep starts one step's cooldown after a ceiling refused its claim.
func (b *agentBusState) noteRefusedStep(claimant, node string) {
	b.refusalMemo().note(claimant, node)
}

// dispatchStartableWork parks the steps this session may hand out, leaving out the ones a ceiling
// just refused: re-parking them every tick is what turned one refusal into a per-tick log pair
// (measured 2026-10-04: refusals=1…11 in six minutes for one unchanged step).
func (b *agentBusState) dispatchStartableWork(ctx context.Context, queueLog *agentbus.QueueLog, st *board.State, now time.Time) error {
	if st == nil {
		return nil
	}
	for _, target := range agentbus.WakeTargets(agentbus.WakeInput{State: st, Now: now}) {
		// Addressed work parks like pooled work: the queue is what the take rule reads,
		// whichever group named the node.
		for _, node := range append(append([]string(nil), target.Ready...), target.Assigned...) {
			if !dispatchable(st, node) || b.refusalMemo().skip(target.Participant, node) {
				continue
			}
			if _, _, err := queueLog.Enqueue(ctx, agentbus.QueueEntry{
				Node: node, Subtree: agentbus.SubtreeRoot(st, node), Participant: target.Participant,
			}, agentbus.QueueLimits{}); err != nil {
				return err
			}
		}
	}
	return nil
}
