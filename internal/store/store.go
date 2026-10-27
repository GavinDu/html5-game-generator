// Package store defines persistence. The memory implementation backs the
// MVP; swap in Postgres by implementing the Store interface.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"doodlequest/internal/model"
)

// clone returns a deep copy of a model value via a JSON round trip.
//
// The store NEVER hands out or keeps a caller's pointer: every value
// crossing the store boundary in either direction is cloned. Pipeline
// goroutines mutate the Game/Job copies they fetched and publish them
// back with Update; HTTP handlers serialize their own copies. Before
// this rule existed, both sides shared one pointer and `go test -race`
// failed in four pipeline/API tests (architecture review 2026-10-23).
// A clone failure (unmarshalable value) falls back to the original —
// model types are plain JSON-shaped data, so this cannot happen in
// practice, and a shallow fallback beats a panic in a request path.
func clone[T any](v *T) *T {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return &out
}

type Artifact struct {
	Data        []byte
	ContentType string
}

type Store interface {
	CreateIngest(r *model.IngestResult)
	GetIngest(id string) (*model.IngestResult, bool)

	CreateGame(g *model.Game)
	GetGame(id string) (*model.Game, bool)
	UpdateGame(g *model.Game)
	ListGames() []*model.Game

	CreateJob(j *model.Job)
	GetJob(id string) (*model.Job, bool)
	UpdateJob(j *model.Job)
	GetJobByGame(gameID string) (*model.Job, bool)

	CreateGroup(gr *model.Group)
	GetGroup(id string) (*model.Group, bool)
	UpdateGroup(gr *model.Group)
	GetGroupByInvite(code string) (*model.Group, bool)

	PutArtifact(gameID, name string, data []byte, contentType string)
	GetArtifact(gameID, name string) ([]byte, string, bool)

	// (The v1 SubmitScore/Leaderboard pair was removed 2026-10-23: it
	// had no callers since the runs-based leaderboard replaced it.)
}

type MemoryStore struct {
	mu         sync.RWMutex
	ingests    map[string]*model.IngestResult
	games      map[string]*model.Game
	jobs       map[string]*model.Job
	jobsByGame map[string]string
	groups     map[string]*model.Group
	artifacts  map[string]map[string]Artifact // gameID -> name -> artifact

	// Optional file persistence (EnablePersistence): snapshot the whole
	// store to one JSON file after every mutation and reload it at
	// boot. Until 2026-10-23 the store was memory-only, so every
	// restart/deploy lost all user-generated games, jobs and groups
	// while social state and run records (separate files) survived —
	// the split the architecture review flagged as P0. The snapshot
	// is whole-state on purpose: at this scale the data is a few
	// hundred KB and correctness beats cleverness; a real database
	// replaces this when volume demands it.
	persistPath           string
	persistenceTargetPath string
	persistenceConfigured bool
	persistenceErr        string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		ingests:    make(map[string]*model.IngestResult),
		games:      make(map[string]*model.Game),
		jobs:       make(map[string]*model.Job),
		jobsByGame: make(map[string]string),
		groups:     make(map[string]*model.Group),
		artifacts:  make(map[string]map[string]Artifact),
	}
}

type storeSnapshot struct {
	Ingests    map[string]*model.IngestResult `json:"ingests"`
	Games      map[string]*model.Game         `json:"games"`
	Jobs       map[string]*model.Job          `json:"jobs"`
	JobsByGame map[string]string              `json:"jobs_by_game"`
	Groups     map[string]*model.Group        `json:"groups"`
	Artifacts  map[string]map[string]Artifact `json:"artifacts"`
}

// EnablePersistence loads the snapshot at path (when one exists) and
// turns on write-through snapshots for every later mutation. A
// corrupt or unreadable snapshot is an error the caller may log and
// continue past in-memory; a missing file is a fresh start.
func (s *MemoryStore) EnablePersistence(path string) error {
	s.mu.Lock()
	s.persistenceConfigured = true
	s.persistenceTargetPath = path
	s.persistenceErr = ""
	s.mu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			s.persistPath = path
			s.persistenceTargetPath = path
			s.mu.Unlock()
			return nil
		}
		s.setPersistenceError(err)
		return err
	}
	var snap storeSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		s.setPersistenceError(err)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if snap.Ingests != nil {
		s.ingests = snap.Ingests
	}
	if snap.Games != nil {
		s.games = snap.Games
	}
	if snap.Jobs != nil {
		s.jobs = snap.Jobs
	}
	if snap.JobsByGame != nil {
		s.jobsByGame = snap.JobsByGame
	}
	if snap.Groups != nil {
		s.groups = snap.Groups
	}
	if snap.Artifacts != nil {
		s.artifacts = snap.Artifacts
	}
	s.persistPath = path
	s.persistenceErr = ""
	return nil
}

// PersistenceStatus reports whether persistence was configured and whether the
// most recent load/write succeeded. It never exposes filesystem paths or errors.
func (s *MemoryStore) PersistenceStatus() (enabled bool, healthy bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.persistenceConfigured, s.persistenceErr == ""
}

func (s *MemoryStore) setPersistenceError(err error) {
	s.mu.Lock()
	s.persistenceErr = err.Error()
	s.mu.Unlock()
}

type backupEnvelope struct {
	FormatVersion int             `json:"formatVersion"`
	SHA256        string          `json:"sha256"`
	Snapshot      json.RawMessage `json:"snapshot"`
}

// BackupTo writes a versioned, checksummed backup containing all durable store
// state, including artifact bytes. It refuses to overwrite the live snapshot.
func (s *MemoryStore) BackupTo(path string) error {
	s.mu.RLock()
	if s.persistenceTargetPath != "" && filepath.Clean(path) == filepath.Clean(s.persistenceTargetPath) {
		s.mu.RUnlock()
		return fmt.Errorf("backup path must differ from live store path")
	}
	snapshot, err := json.Marshal(storeSnapshot{
		Ingests: s.ingests, Games: s.games, Jobs: s.jobs,
		JobsByGame: s.jobsByGame, Groups: s.groups, Artifacts: s.artifacts,
	})
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("encode store snapshot: %w", err)
	}
	sum := sha256.Sum256(snapshot)
	encoded, err := json.Marshal(backupEnvelope{
		FormatVersion: 1, SHA256: hex.EncodeToString(sum[:]), Snapshot: snapshot,
	})
	if err != nil {
		return fmt.Errorf("encode backup envelope: %w", err)
	}
	return writeAtomicFile(path, encoded)
}

// RestoreFrom validates a backup before changing memory or the live snapshot.
// When persistence is configured, the restored snapshot is atomically written
// to its configured path before it becomes the active in-memory state.
func (s *MemoryStore) RestoreFrom(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}
	var envelope backupEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode backup envelope: %w", err)
	}
	if envelope.FormatVersion != 1 {
		return fmt.Errorf("unsupported backup format version %d", envelope.FormatVersion)
	}
	sum := sha256.Sum256(envelope.Snapshot)
	if envelope.SHA256 != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("backup checksum mismatch")
	}
	var snapshot storeSnapshot
	if err := json.Unmarshal(envelope.Snapshot, &snapshot); err != nil {
		return fmt.Errorf("decode backup snapshot: %w", err)
	}
	if snapshot.Ingests == nil || snapshot.Games == nil || snapshot.Jobs == nil ||
		snapshot.JobsByGame == nil || snapshot.Groups == nil || snapshot.Artifacts == nil {
		return fmt.Errorf("backup snapshot is missing required collections")
	}
	if err := validateStoreSnapshot(snapshot); err != nil {
		return fmt.Errorf("invalid backup snapshot: %w", err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode restored snapshot: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.persistenceConfigured {
		target := s.persistenceTargetPath
		if target == "" {
			return fmt.Errorf("persistence target path is unavailable")
		}
		if err := writeAtomicFile(target, encoded); err != nil {
			s.persistenceErr = err.Error()
			return fmt.Errorf("write restored snapshot: %w", err)
		}
		s.persistPath = target
	}
	s.ingests = snapshot.Ingests
	s.games = snapshot.Games
	s.jobs = snapshot.Jobs
	s.jobsByGame = snapshot.JobsByGame
	s.groups = snapshot.Groups
	s.artifacts = snapshot.Artifacts
	s.persistenceErr = ""
	return nil
}

func writeAtomicFile(path string, data []byte) error {
	if path == "" {
		return fmt.Errorf("file path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// persistLocked writes the snapshot; callers hold s.mu. Failures are
// swallowed by design (same posture as the social state file): a disk
// problem must not take request handling down with it.
func (s *MemoryStore) persistLocked() {
	if s.persistPath == "" {
		return
	}
	data, err := json.Marshal(storeSnapshot{
		Ingests: s.ingests, Games: s.games, Jobs: s.jobs,
		JobsByGame: s.jobsByGame, Groups: s.groups, Artifacts: s.artifacts,
	})
	if err != nil {
		s.persistenceErr = err.Error()
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.persistPath), 0o700); err != nil {
		s.persistenceErr = err.Error()
		return
	}
	tmp := s.persistPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		s.persistenceErr = err.Error()
		return
	}
	if err := os.Rename(tmp, s.persistPath); err != nil {
		s.persistenceErr = err.Error()
		return
	}
	s.persistenceErr = ""
}

func (s *MemoryStore) CreateIngest(r *model.IngestResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ingests[r.IngestID] = clone(r)
	s.persistLocked()
}

func (s *MemoryStore) GetIngest(id string) (*model.IngestResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.ingests[id]
	return clone(r), ok
}

func (s *MemoryStore) CreateGame(g *model.Game) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.games[g.ID] = clone(g)
	s.persistLocked()
}

func (s *MemoryStore) GetGame(id string) (*model.Game, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	return clone(g), ok
}

func (s *MemoryStore) UpdateGame(g *model.Game) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.games[g.ID] = clone(g)
	s.persistLocked()
}

func (s *MemoryStore) ListGames() []*model.Game {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Game, 0, len(s.games))
	for _, g := range s.games {
		out = append(out, clone(g))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *MemoryStore) CreateJob(j *model.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.ID] = clone(j)
	s.jobsByGame[j.GameID] = j.ID
	s.persistLocked()
}

func (s *MemoryStore) GetJob(id string) (*model.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	return clone(j), ok
}

func (s *MemoryStore) UpdateJob(j *model.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.ID] = clone(j)
	s.persistLocked()
}

func (s *MemoryStore) GetJobByGame(gameID string) (*model.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.jobsByGame[gameID]
	if !ok {
		return nil, false
	}
	j, ok := s.jobs[id]
	return clone(j), ok
}

func (s *MemoryStore) CreateGroup(gr *model.Group) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups[gr.ID] = clone(gr)
	s.persistLocked()
}

func (s *MemoryStore) GetGroup(id string) (*model.Group, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	gr, ok := s.groups[id]
	return clone(gr), ok
}

func (s *MemoryStore) UpdateGroup(gr *model.Group) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups[gr.ID] = clone(gr)
	s.persistLocked()
}

func (s *MemoryStore) GetGroupByInvite(code string) (*model.Group, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, gr := range s.groups {
		if gr.InviteCode == code {
			return clone(gr), true
		}
	}
	return nil, false
}

func (s *MemoryStore) PutArtifact(gameID, name string, data []byte, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.artifacts[gameID] == nil {
		s.artifacts[gameID] = make(map[string]Artifact)
	}
	s.artifacts[gameID][name] = Artifact{Data: data, ContentType: contentType}
	s.persistLocked()
}

func (s *MemoryStore) GetArtifact(gameID, name string) ([]byte, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.artifacts[gameID]
	if !ok {
		return nil, "", false
	}
	a, ok := m[name]
	if !ok {
		return nil, "", false
	}
	return a.Data, a.ContentType, true
}
