// Package store defines persistence. The memory implementation backs the
// MVP; swap in Postgres by implementing the Store interface.
package store

import (
	"encoding/json"
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
	persistPath string
	persistenceConfigured bool
	persistenceErr string
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
	s.persistenceErr = ""
	s.mu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			s.persistPath = path
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
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
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
