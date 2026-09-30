// Package recap keeps one recap per closed session: the reusable notes (facts,
// root causes, refutations, handoffs) the next session can build on instead of
// re-deriving them. A note stays a candidate until a person accepts it, so this
// package never writes memory itself.
//
// A recap is a disposable projection of the authoritative transcript, never a
// replacement for it. It is produced by one independent bounded call that does
// not enter the session, its prompt cache, or its usage accounting: the recap
// lane has its own provider instance, its own usage source, and its own
// concurrency gate, and it yields to sessions that are running.
package recap
