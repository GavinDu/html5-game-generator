package attestation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	localJournalFormat     = 1
	maxLocalJournalBytes   = 8 << 20
	maxLocalJournalRecords = 10000
	maxJournalJobIDBytes   = 128
)

var (
	ErrJournalConflict    = errors.New("server verdict journal conflict")
	ErrJournalUnavailable = errors.New("server verdict journal unavailable")
)

// LocalVerdictJournal is a single-process, private file-backed compare-and-
// swap boundary. Its caller MUST be an independently authenticated server
// replay worker; these Go methods do not authenticate any actor. Do NOT
// expose it directly through HTTP or use it as a distributed worker ledger.
type LocalVerdictJournal struct {
	mu      sync.Mutex
	path    string
	records map[string]journalEntry
	// A rename succeeded but the directory sync failed. The on-disk state
	// may be newer than memory; block further writes until reopened.
	uncertain bool
}

type journalEntry struct {
	Verdict    Verdict `json:"verdict"`
	FinalJobID string  `json:"finalJobId,omitempty"`
}

type localJournalSnapshot struct {
	Version int                     `json:"version"`
	Records map[string]journalEntry `json:"records"`
}

type localJournalEnvelope struct {
	FormatVersion int             `json:"formatVersion"`
	SHA256        string          `json:"sha256"`
	Snapshot      json.RawMessage `json:"snapshot"`
}

func validJournalJobID(id string) bool {
	if len(id) == 0 || len(id) > maxJournalJobIDBytes {
		return false
	}
	for _, ch := range id {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') &&
			(ch < '0' || ch > '9') && ch != '-' && ch != '_' && ch != '.' {
			return false
		}
	}
	return true
}

func strictJournalJSON(data []byte, into any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("trailing journal content")
	}
	return nil
}

func validateJournalEntry(runID string, entry journalEntry) error {
	if runID == "" || runID != entry.Verdict.Identity.RunID {
		return ErrJournalConflict
	}
	pending, err := NewPending(entry.Verdict.Identity)
	if err != nil {
		return err
	}
	switch entry.Verdict.Status {
	case Pending:
		if entry.FinalJobID != "" || entry.Verdict != pending {
			return ErrJournalConflict
		}
	case Verified, Rejected:
		if !validJournalJobID(entry.FinalJobID) {
			return ErrJournalConflict
		}
		next := Decision{
			Identity:        entry.Verdict.Identity,
			Status:          entry.Verdict.Status,
			Score:           entry.Verdict.Score,
			Won:             entry.Verdict.Won,
			StateSHA256:     entry.Verdict.StateSHA256,
			RejectionReason: entry.Verdict.RejectionReason,
		}
		recomputed, err := ApplyServerDecision(pending, next)
		if err != nil || recomputed != entry.Verdict {
			return ErrJournalConflict
		}
	default:
		return ErrJournalConflict
	}
	return nil
}

// OpenLocalVerdictJournal loads a versioned, checksum-protected journal.
// A damaged or inaccessible existing journal is a HARD error; never reset
// its verified verdicts to empty. The path must be explicitly absolute.
func OpenLocalVerdictJournal(path string) (*LocalVerdictJournal, error) {
	if !filepath.IsAbs(path) || strings.TrimSpace(path) != path {
		return nil, ErrJournalUnavailable
	}
	path = filepath.Clean(path)
	j := &LocalVerdictJournal{path: path, records: make(map[string]journalEntry)}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return j, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrJournalUnavailable
	}
	// Use a bounded reader as well as lstat: a damaged journal must never
	// allocate unbounded memory if it grows between metadata and the read.
	if info.Size() > maxLocalJournalBytes {
		return nil, ErrJournalUnavailable
	}
	opened, err := os.Open(path)
	if err != nil {
		return nil, ErrJournalUnavailable
	}
	openedInfo, statErr := opened.Stat()
	if statErr != nil || !openedInfo.Mode().IsRegular() ||
		openedInfo.Mode().Perm()&0077 != 0 {
		_ = opened.Close()
		return nil, ErrJournalUnavailable
	}
	data, readErr := io.ReadAll(io.LimitReader(opened, maxLocalJournalBytes+1))
	closeErr := opened.Close()
	if readErr != nil || closeErr != nil || len(data) > maxLocalJournalBytes {
		return nil, ErrJournalUnavailable
	}
	var env localJournalEnvelope
	if err := strictJournalJSON(data, &env); err != nil ||
		env.FormatVersion != localJournalFormat || len(env.SHA256) != 64 {
		return nil, ErrJournalUnavailable
	}
	digest := sha256.Sum256(env.Snapshot)
	if hex.EncodeToString(digest[:]) != env.SHA256 {
		return nil, ErrJournalUnavailable
	}
	var snap localJournalSnapshot
	if err := strictJournalJSON(env.Snapshot, &snap); err != nil ||
		snap.Version != localJournalFormat || snap.Records == nil ||
		len(snap.Records) > maxLocalJournalRecords {
		return nil, ErrJournalUnavailable
	}
	// A server replay worker job must never finalize two different attempts.
	// Reject preexisting aliasing on restart rather than trusting a snapshot
	// with ambiguous idempotency ownership.
	jobOwners := make(map[string]string, len(snap.Records))
	for runID, entry := range snap.Records {
		if err := validateJournalEntry(runID, entry); err != nil {
			return nil, ErrJournalUnavailable
		}
		if entry.FinalJobID != "" {
			if owner, exists := jobOwners[entry.FinalJobID]; exists && owner != runID {
				return nil, ErrJournalUnavailable
			}
			jobOwners[entry.FinalJobID] = runID
		}
	}
	j.records = snap.Records
	return j, nil
}

func (j *LocalVerdictJournal) persist(next map[string]journalEntry) error {
	if j.uncertain {
		return ErrJournalUnavailable
	}
	if len(next) > maxLocalJournalRecords {
		return ErrJournalUnavailable
	}
	snapshot, err := json.Marshal(localJournalSnapshot{
		Version: localJournalFormat, Records: next,
	})
	if err != nil {
		return fmt.Errorf("%w: encode", ErrJournalUnavailable)
	}
	digest := sha256.Sum256(snapshot)
	data, err := json.Marshal(localJournalEnvelope{
		FormatVersion: localJournalFormat,
		SHA256:        hex.EncodeToString(digest[:]),
		Snapshot:      snapshot,
	})
	if err != nil || len(data) > maxLocalJournalBytes {
		return ErrJournalUnavailable
	}
	dir := filepath.Dir(j.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return ErrJournalUnavailable
	}
	tmp, err := os.CreateTemp(dir, ".server-verdict-*")
	if err != nil {
		return ErrJournalUnavailable
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return ErrJournalUnavailable
	}
	_, writeErr := tmp.Write(data)
	if writeErr == nil {
		writeErr = tmp.Sync()
	}
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		return ErrJournalUnavailable
	}
	if err := os.Rename(tmpPath, j.path); err != nil {
		return ErrJournalUnavailable
	}
	d, err := os.Open(dir)
	if err != nil {
		j.uncertain = true
		return ErrJournalUnavailable
	}
	syncErr := d.Sync()
	closeErr = d.Close()
	if syncErr != nil || closeErr != nil {
		j.uncertain = true
		return ErrJournalUnavailable
	}
	return nil
}

// CreatePending persists only an immutable *server-issued* identity, with
// zero verified score. An existing run ID with a different identity conflicts.
func (j *LocalVerdictJournal) CreatePending(id Identity) (Verdict, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.uncertain {
		return Verdict{}, ErrJournalUnavailable
	}
	pending, err := NewPending(id)
	if err != nil {
		return Verdict{}, err
	}
	if existing, ok := j.records[id.RunID]; ok {
		if existing.Verdict.Identity != id {
			return Verdict{}, ErrJournalConflict
		}
		return existing.Verdict, nil
	}
	next := make(map[string]journalEntry, len(j.records)+1)
	for k, v := range j.records {
		next[k] = v
	}
	next[id.RunID] = journalEntry{Verdict: pending}
	if err := j.persist(next); err != nil {
		return Verdict{}, err
	}
	j.records = next
	return pending, nil
}

// CommitServerDecision accepts a final verdict only from a future private
// replay worker. It validates a bounded job ID and the pure reducer before
// persisting the entire next snapshot, then publishes the in-memory change.
// Identical job+outcome is idempotent across reopen; even an identical
// outcome from another job is rejected to prevent double side effects.
func (j *LocalVerdictJournal) CommitServerDecision(jobID string, decision Decision) (Verdict, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.uncertain {
		return Verdict{}, ErrJournalUnavailable
	}
	if !validJournalJobID(jobID) {
		return Verdict{}, ErrInvalidDecision
	}
	runID := decision.Identity.RunID
	current, ok := j.records[runID]
	if !ok {
		return Verdict{}, ErrJournalConflict
	}
	// Bound to maxLocalJournalRecords by admission: a job ID is a unique
	// idempotency identity for one run, never a transferable capability.
	for existingRunID, entry := range j.records {
		if existingRunID != runID && entry.FinalJobID == jobID {
			return Verdict{}, ErrJournalConflict
		}
	}
	output, err := ApplyServerDecision(current.Verdict, decision)
	if err != nil {
		return Verdict{}, err
	}
	if current.Verdict.Status != Pending {
		if current.FinalJobID != jobID {
			return Verdict{}, ErrJournalConflict
		}
		return output, nil
	}
	next := make(map[string]journalEntry, len(j.records))
	for k, v := range j.records {
		next[k] = v
	}
	next[runID] = journalEntry{Verdict: output, FinalJobID: jobID}
	if err := j.persist(next); err != nil {
		return Verdict{}, err
	}
	j.records = next
	return output, nil
}

func (j *LocalVerdictJournal) Read(runID string) (Verdict, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.uncertain {
		return Verdict{}, false
	}
	entry, ok := j.records[runID]
	return entry.Verdict, ok
}
