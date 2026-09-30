package recap

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Insight is one conclusion more than one project reached on its own. It is the
// one claim a recap can make that a single session cannot: not "I found this" but
// "this keeps being true".
//
// The evidence is the offer rule's own (two shared identifiers, or three shared
// character pairs), so an insight is exactly as checkable as the tier proposal a
// note carries — and, like everything else here, it costs no model calls.
type Insight struct {
	Kind     string
	Body     string
	Evidence string
	// Projects names every project that reached it, so "how many" is checkable
	// without opening the projection.
	Projects []string
	SeenAt   time.Time
}

// Insights returns the conclusions at least two projects reached independently,
// newest first, limited to those seen since the given time.
//
// Two projects state one conclusion in their own words, so copies that share the
// same evidence are folded together: the report is about what recurred, not about
// how many times someone phrased it.
func (s *Store) Insights(ctx context.Context, since time.Time, limit int) ([]Insight, error) {
	if s == nil || s.handle == nil {
		return nil, nil
	}
	records, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	recurrences, err := s.Recurrences(ctx)
	if err != nil {
		return nil, err
	}
	type copyOfInsight struct {
		insight Insight
		ids     []string
	}
	raw := []copyOfInsight{}
	for _, record := range records {
		if ProjectOf(record.Path) == "" {
			continue
		}
		for _, entry := range record.Entries {
			if strings.TrimSpace(entry.Body) == "" {
				continue
			}
			buckets := recurrences[HashEntry(entry.Kind, entry.Body)]
			if len(buckets) < 2 {
				continue
			}
			raw = append(raw, copyOfInsight{
				insight: Insight{Kind: entry.Kind, Body: entry.Body, Evidence: entry.Evidence,
					Projects: labels(buckets), SeenAt: record.GeneratedAt},
				ids: unique(entryTokens(entry).identifiers),
			})
		}
	}

	merged := []copyOfInsight{}
	for _, candidate := range raw {
		placed := false
		for i := range merged {
			if merged[i].insight.Kind != candidate.insight.Kind {
				continue
			}
			if sharedEvidenceCount(merged[i].ids, candidate.ids) < sharedPairsFloor {
				continue
			}
			merged[i].insight.Projects = unionLabels(merged[i].insight.Projects, candidate.insight.Projects)
			if len(candidate.insight.Evidence) > len(merged[i].insight.Evidence) {
				merged[i].insight.Evidence = candidate.insight.Evidence
			}
			if candidate.insight.SeenAt.After(merged[i].insight.SeenAt) {
				merged[i].insight.Body = candidate.insight.Body
				merged[i].insight.SeenAt = candidate.insight.SeenAt
			}
			merged[i].ids = unionTokens(merged[i].ids, candidate.ids)
			placed = true
			break
		}
		if !placed {
			merged = append(merged, candidate)
		}
	}

	out := make([]Insight, 0, len(merged))
	for _, item := range merged {
		if !since.IsZero() && item.insight.SeenAt.Before(since) {
			continue
		}
		out = append(out, item.insight)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SeenAt.After(out[j].SeenAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func labels(buckets []string) []string {
	out := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		if label := BucketLabel(bucket); label != "" {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

// sharedEvidenceCount counts the identifiers two copies of a conclusion have in
// common — the same evidence that made them a match in the first place.
func sharedEvidenceCount(a, b []string) int {
	seen := make(map[string]bool, len(a))
	for _, id := range a {
		seen[id] = true
	}
	count := 0
	for _, id := range b {
		if seen[id] {
			count++
		}
	}
	return count
}

func unionTokens(a, b []string) []string {
	return unique(append(append([]string{}, a...), b...))
}

func unionLabels(a, b []string) []string {
	return labels(unique(append(append([]string{}, a...), b...)))
}
