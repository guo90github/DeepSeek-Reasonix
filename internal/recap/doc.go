// Package recap keeps one recap per closed session: the four elements (goal,
// key actions, conclusion, follow-ups) that answer "what did I do here".
//
// A recap is a disposable projection of the authoritative transcript, never a
// replacement for it. It is produced by one independent bounded call that does
// not enter the session, its prompt cache, or its usage accounting: the recap
// lane has its own provider instance, its own usage source, and its own
// concurrency gate, and it yields to sessions that are running.
package recap
