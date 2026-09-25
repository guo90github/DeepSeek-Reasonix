// Package collabgate keeps the collaboration surface and the code it points at
// from drifting apart. It is one gate with two readings: every `文件:行` anchor
// in docs/COLLAB-SURFACE.md must still resolve to a token its own row claims
// (coordinates), and the machine-readable names the docs promise must exist in
// code while every docs/*.md path cited from non-test Go source must exist on
// disk (document ⇄ code, both directions). Dangling means red: a drifted
// coordinate or a citation to a doc that was never written is exactly what
// these two readings exist to catch, instead of a human noticing it later.
package collabgate
