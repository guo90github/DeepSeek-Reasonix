// Package doccmdgate keeps a documented verification command from lying. A
// `go test -run '<pattern>'` written in a doc is an instruction someone will
// paste; when the pattern selects no test at all, `go test` still exits 0 and
// the reader believes the suite passed. This repo has done exactly that: a
// handoff recorded `-run 'Wake\|Dispatch'` as green while it ran nothing,
// because `\|` is a grep-ism that RE2 reads as a literal pipe.
//
// One reading, two rules, checked by scanning the tree — no network, no build:
//
//   - every `-run` pattern in a scanned markdown file must match at least one
//     test function declared in this repo;
//   - a pattern may not contain `\|`, and it must compile as a regexp.
//
// Scanned: markdown under the repo root, minus generated trees and dated round
// logs (`logs/`), which record what was once run instead of instructing anyone.
// `-run '^$'` is allowed on purpose: it is the documented way to build a test
// binary without running anything.
//
// The check is deliberately static: resolving which *package* a pattern is run
// in would mean building every package. A pattern that names a test living in
// another module or package is out of reach of this reading.
package doccmdgate
