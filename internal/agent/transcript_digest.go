package agent

import "reasonix/internal/provider"

// TranscriptDigest reports the stored-content digest of msgs: the value Save
// records in a session's sidecar. A reader that assembles the same messages by
// another route can compare against it to prove the two agree.
func TranscriptDigest(msgs []provider.Message) string {
	hasher := newSessionTranscriptHasher()
	hasher.addAll(msgs)
	digest, ok := hasher.sum()
	if !ok {
		return ""
	}
	return digestString(digest)
}

// SessionContentDigest reads the digest a session sidecar recorded. ok is false
// for a session that has never recorded one.
func SessionContentDigest(path string) (digest string, ok bool, err error) {
	_, digest, err = sessionContentRevision(path)
	if err != nil {
		return "", false, err
	}
	if digest == "" {
		return "", false, nil
	}
	return digest, true, nil
}
