package agentbus

import "testing"

// A verification wake is still one assignment — the prompt and the line builders branch on the
// dispatch prefix — so the verify key has to keep it while differing from the plain one: the wake
// ledger collapses repeats by key, and a verification swallowed as "already told" is the waste the
// intent exists to remove (F15, 2026-10-05).
func TestAVerifyKeyIsADispatchKeyThatDiffersFromThePlainOne(t *testing.T) {
	plain := DispatchKey("board", "step")
	verify := VerifyKey("board", "step")

	if !IsDispatchKey(verify) {
		t.Fatalf("VerifyKey(%q) does not read as a dispatch key", verify)
	}
	if verify == plain {
		t.Fatalf("VerifyKey and DispatchKey are both %q: the ledger cannot tell them apart", verify)
	}
}
