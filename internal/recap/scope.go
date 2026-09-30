package recap

import (
	"context"
	"sort"
	"strings"
)

// Recurrences maps each note to the buckets that reached a conclusion like it.
// The model proposes a deposition tier, and this is what makes the proposal
// checkable instead of taken on faith: a note that only one project ever produced
// cannot claim to be about work in general, no matter what it says about itself.
//
// It is one pass over the projection — matching is the offer rule's own evidence
// and thresholds (two shared identifiers, or three shared character pairs) — so a
// page listing every recap pays for the whole answer once.
func (s *Store) Recurrences(ctx context.Context) (map[string][]string, error) {
	if s == nil || s.handle == nil {
		return nil, nil
	}
	records, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	byIdentifier := map[string]map[string]bool{}
	byPair := map[string]map[string]bool{}
	add := func(index map[string]map[string]bool, token, bucket string) {
		if index[token] == nil {
			index[token] = map[string]bool{}
		}
		index[token][bucket] = true
	}
	for _, record := range records {
		bucket := ProjectOf(record.Path)
		if bucket == "" {
			continue
		}
		for _, entry := range record.Entries {
			tokens := entryTokens(entry)
			for _, identifier := range tokens.identifiers {
				add(byIdentifier, identifier, bucket)
			}
			for _, pair := range unique(tokens.pairs) {
				add(byPair, pair, bucket)
			}
		}
	}

	out := map[string][]string{}
	for _, record := range records {
		if ProjectOf(record.Path) == "" {
			continue
		}
		for _, entry := range record.Entries {
			tokens := entryTokens(entry)
			identifiers := map[string]int{}
			for _, identifier := range tokens.identifiers {
				for bucket := range byIdentifier[identifier] {
					identifiers[bucket]++
				}
			}
			pairs := map[string]int{}
			for _, pair := range unique(tokens.pairs) {
				for bucket := range byPair[pair] {
					pairs[bucket]++
				}
			}
			seen := map[string]bool{}
			buckets := []string{}
			for bucket, n := range identifiers {
				if n >= sharedIdentifiersNeeded {
					seen[bucket] = true
					buckets = append(buckets, bucket)
				}
			}
			for bucket, n := range pairs {
				if n >= sharedPairsAlone && !seen[bucket] {
					buckets = append(buckets, bucket)
				}
			}
			if len(buckets) == 0 {
				continue
			}
			sort.Strings(buckets)
			out[HashEntry(entry.Kind, entry.Body)] = buckets
		}
	}
	return out, nil
}

// ObservationOf reports the labelled buckets a note was also concluded in, its own
// bucket left out: what a person needs to see before calling a note general.
func ObservationOf(recurrences []string, own string) []string {
	out := []string{}
	for _, bucket := range recurrences {
		if bucket == own {
			continue
		}
		out = append(out, BucketLabel(bucket))
	}
	return out
}

// BucketLabel names a bucket the way a person reads it: the last segment of the
// path, which for a project directory is the workspace it belongs to.
func BucketLabel(bucket string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(bucket), `/\`)
	if trimmed == "" {
		return ""
	}
	if idx := strings.LastIndexAny(trimmed, `/\`); idx >= 0 {
		return trimmed[idx+1:]
	}
	return trimmed
}

// entryTokens is the evidence one note offers, the same way an unfinished item's
// is read.
func entryTokens(entry Entry) tokens { return tokenize(entry.Body + " " + entry.Evidence) }

// unique keeps the first occurrence of each token: a repeated pair must not count
// twice towards the threshold that makes a subject.
func unique(tokens []string) []string {
	seen := make(map[string]bool, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}
