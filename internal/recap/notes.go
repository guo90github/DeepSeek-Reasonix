package recap

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Note is one distilled conclusion as the turn tail reads it back: what was
// concluded, why it is trusted, and which session concluded it.
type Note struct {
	Kind        string
	Body        string
	Evidence    string
	From        string
	GeneratedAt time.Time
}

// Offers is what one project carries into a turn: unfinished items worth asking
// the person about, and earlier conclusions worth carrying as background. They
// arrive together because both come from the same projection and are judged
// against the same turn.
type Offers struct {
	Items []OpenItem
	Prior []Note
}

// priorNoteKinds are the conclusions worth carrying into a later session as
// background: what was ruled out, and why something turned out the way it did.
// Facts belong in the memory the person accepted them into, and handoff notes
// have their own channel — that one asks, this one does not.
var priorNoteKinds = []string{KindRefuted, KindRootCause}

// PriorNotes returns one bucket's background conclusions, newest first. Notes the
// person already decided on are left out: an accepted note is already in memory,
// and a rejected one must never come back — that is the promise the drop button
// makes.
func (s *Store) PriorNotes(ctx context.Context, bucket string, limit int) ([]Note, error) {
	bucket = strings.TrimSpace(bucket)
	if s == nil || s.handle == nil || bucket == "" {
		return nil, nil
	}
	records, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	decisions, err := s.Decisions(ctx)
	if err != nil {
		decisions = map[string]Decision{}
	}
	wanted := make(map[string]bool, len(priorNoteKinds))
	for _, kind := range priorNoteKinds {
		wanted[kind] = true
	}
	out := []Note{}
	for _, rec := range records {
		if ProjectOf(rec.Path) != bucket {
			continue
		}
		for _, entry := range rec.Entries {
			if !wanted[entry.Kind] || strings.TrimSpace(entry.Body) == "" {
				continue
			}
			if _, decided := decisions[HashEntry(entry.Kind, entry.Body)]; decided {
				continue
			}
			out = append(out, Note{
				Kind:        entry.Kind,
				Body:        entry.Body,
				Evidence:    entry.Evidence,
				From:        rec.Path,
				GeneratedAt: rec.GeneratedAt,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].GeneratedAt.After(out[j].GeneratedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MatchNotes returns the notes a turn appears to be about, capped at limit. A note
// is judged by the same token evidence an unfinished item is: the same subject,
// said differently, is still the same subject.
func MatchNotes(notes []Note, turn string, limit int) []Note {
	return matchNotes(notes, turn, limit, tokens.matches)
}

// StrongMatchNotes is the mid-conversation test: a turn that says nothing about
// picking something up has to name the note's own things.
func StrongMatchNotes(notes []Note, turn string, limit int) []Note {
	return matchNotes(notes, turn, limit, tokens.names)
}

func matchNotes(notes []Note, turn string, limit int, about func(tokens, string) bool) []Note {
	lower := strings.ToLower(turn)
	out := []Note{}
	for _, note := range notes {
		if !about(tokenize(note.Body+" "+note.Evidence), lower) {
			continue
		}
		out = append(out, note)
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out
}
