package agentbus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"reasonix/internal/agentbus/jsonl"
	"reasonix/internal/filelock"
)

const (
	directoryName = "participants.jsonl"
	directoryLock = "participants.jsonl.lock"
	directoryWait = 5 * time.Second
)

// ParticipantRef is where one participant speaks from: the host that can deliver to
// it, the session it must be addressed as, and the file holding that host's token.
// A wake names a participant, so routing one across processes needs this address
// (AGENT_BUS §13.3, second gap).
type ParticipantRef struct {
	Participant string `json:"participant"`
	Host        string `json:"host,omitempty"`
	SessionPath string `json:"sessionPath,omitempty"`
	TokenFile   string `json:"tokenFile,omitempty"`
	// The declared dimensions: what this participant is and what it speaks as, so a reader does
	// not have to guess from an opaque id (F48, 2026-10-05). Role stays empty until a host
	// declares one: the kernel has no vocabulary for it and invents none.
	Workspace string    `json:"workspace,omitempty"`
	Model     string    `json:"model,omitempty"`
	Role      string    `json:"role,omitempty"`
	Withdrawn bool      `json:"withdrawn,omitempty"`
	At        time.Time `json:"at,omitempty"`
}

// ParticipantTTL is how long an announcement stays live.
//
// Hosts renew by re-announcing (the headless tick does it every 30s), so a live participant
// stays well inside this window; a host that is gone cannot, and an address nobody renews must
// stop being addressable rather than accept wakes it will never look at (AGENT_BUS §11.5).
const ParticipantTTL = 30 * time.Minute

// Stale reports whether an announcement is older than the TTL. A record without a timestamp is
// left alone: an unstamped line is malformed input, not evidence that a host went away.
func (r ParticipantRef) Stale(now time.Time) bool {
	if r.At.IsZero() {
		return false
	}
	return now.Sub(r.At) > ParticipantTTL
}

// Refusal reasons for an announcement.
const (
	RefuseDirectoryNoParticipant = "directory_participant_missing"
	RefuseDirectoryTokenNotFile  = "directory_token_not_a_file"
)

// DirectoryReject explains a refused announcement.
type DirectoryReject struct {
	Participant string
	Reason      string
}

func (e *DirectoryReject) Error() string {
	return fmt.Sprintf("agentbus directory: %q refused: %s", e.Participant, e.Reason)
}

// IsDirectoryReject reports whether err is a directory refusal and returns its reason.
func IsDirectoryReject(err error) (string, bool) {
	var rej *DirectoryReject
	if !errors.As(err, &rej) {
		return "", false
	}
	return rej.Reason, true
}

// ParticipantDirectory is the board's address book: an append-only log of
// announcements beside the op log, folded to the newest address per participant.
// Appends rather than read-modify-write, so two hosts announcing at once cannot
// clobber each other.
type ParticipantDirectory struct {
	boardDir string
}

// OpenParticipantDirectory addresses the directory stored alongside a board.
func OpenParticipantDirectory(boardDir string) (*ParticipantDirectory, error) {
	if strings.TrimSpace(boardDir) == "" {
		return nil, fmt.Errorf("agentbus: empty board directory")
	}
	abs, err := filepath.Abs(boardDir)
	if err != nil {
		return nil, fmt.Errorf("agentbus: resolve board directory: %w", err)
	}
	if err := jsonl.EnsureDir(abs); err != nil {
		return nil, err
	}
	return &ParticipantDirectory{boardDir: abs}, nil
}

// Path is the file the announcements land in.
func (d *ParticipantDirectory) Path() string { return filepath.Join(d.boardDir, directoryName) }

// Announce publishes where this participant speaks from. A later announcement for
// the same participant replaces the earlier one; that is how a session that moved
// hosts updates its address.
func (d *ParticipantDirectory) Announce(ctx context.Context, ref ParticipantRef) (ParticipantRef, error) {
	ref.Participant = strings.TrimSpace(ref.Participant)
	if err := validateParticipantRef(ref); err != nil {
		return ParticipantRef{}, err
	}
	ref.Host = strings.TrimSpace(ref.Host)
	ref.SessionPath = strings.TrimSpace(ref.SessionPath)
	ref.TokenFile = strings.TrimSpace(ref.TokenFile)
	if ref.At.IsZero() {
		ref.At = time.Now().UTC()
	}
	release, err := filelock.AcquireWithExternalTimeout(ctx, filepath.Join(d.boardDir, directoryLock), directoryWait)
	if err != nil {
		return ParticipantRef{}, err
	}
	defer release()
	if err := jsonl.Append(d.Path(), ref); err != nil {
		return ParticipantRef{}, err
	}
	return ref, nil
}

// Withdraw removes an address: a session that left the board must not keep being
// woken somewhere it no longer is. The record stays, so the removal is visible.
func (d *ParticipantDirectory) Withdraw(ctx context.Context, participant string) error {
	_, err := d.Announce(ctx, ParticipantRef{
		Participant: participant, Withdrawn: true, At: time.Now().UTC(),
	})
	return err
}

// Lookup reports where one participant speaks from. A withdrawn participant is
// reported as absent, not as an address that no longer works.
func (d *ParticipantDirectory) Lookup(participant string) (ParticipantRef, bool, error) {
	refs, err := d.All()
	if err != nil {
		return ParticipantRef{}, false, err
	}
	for _, ref := range refs {
		if ref.Participant == strings.TrimSpace(participant) {
			return ref, true, nil
		}
	}
	return ParticipantRef{}, false, nil
}

// All lists the live addresses, sorted by participant so two readers agree.
func (d *ParticipantDirectory) All() ([]ParticipantRef, error) {
	read, err := jsonl.ReadAll[ParticipantRef](d.Path())
	if err != nil {
		return nil, err
	}
	newest := map[string]ParticipantRef{}
	order := []string{}
	for _, ref := range read.Items {
		participant := strings.TrimSpace(ref.Participant)
		if participant == "" {
			continue
		}
		if _, seen := newest[participant]; !seen {
			order = append(order, participant)
		}
		newest[participant] = ref
	}
	out := make([]ParticipantRef, 0, len(order))
	for _, participant := range order {
		ref := newest[participant]
		if ref.Withdrawn || ref.Stale(time.Now().UTC()) {
			continue
		}
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Participant < out[j].Participant })
	return out, nil
}

// validateParticipantRef keeps an announcement addressable: a token that is not a
// file cannot be read at delivery time, so it is refused when it is announced
// rather than discovered as a failed wake later.
func validateParticipantRef(ref ParticipantRef) error {
	if ref.Participant == "" {
		return &DirectoryReject{Participant: ref.Participant, Reason: RefuseDirectoryNoParticipant}
	}
	token := strings.TrimSpace(ref.TokenFile)
	if token == "" {
		return nil
	}
	if info, err := os.Stat(token); err != nil || info.IsDir() {
		return &DirectoryReject{Participant: ref.Participant, Reason: RefuseDirectoryTokenNotFile}
	}
	return nil
}
