// Package store defines persistence. The memory implementation backs the
// MVP; swap in Postgres by implementing the Store interface.
package store

import (
	"sort"
	"sync"

	"doodlequest/internal/model"
)

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
}

type MemoryStore struct {
	mu         sync.RWMutex
	ingests    map[string]*model.IngestResult
	games      map[string]*model.Game
	jobs       map[string]*model.Job
	jobsByGame map[string]string
	groups     map[string]*model.Group
	artifacts  map[string]map[string]Artifact // gameID -> name -> artifact
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

func (s *MemoryStore) CreateIngest(r *model.IngestResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ingests[r.IngestID] = r
}

func (s *MemoryStore) GetIngest(id string) (*model.IngestResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.ingests[id]
	return r, ok
}

func (s *MemoryStore) CreateGame(g *model.Game) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.games[g.ID] = g
}

func (s *MemoryStore) GetGame(id string) (*model.Game, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.games[id]
	return g, ok
}

func (s *MemoryStore) UpdateGame(g *model.Game) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.games[g.ID] = g
}

func (s *MemoryStore) ListGames() []*model.Game {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Game, 0, len(s.games))
	for _, g := range s.games {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MemoryStore) CreateJob(j *model.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.ID] = j
	s.jobsByGame[j.GameID] = j.ID
}

func (s *MemoryStore) GetJob(id string) (*model.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	return j, ok
}

func (s *MemoryStore) UpdateJob(j *model.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.ID] = j
}

func (s *MemoryStore) GetJobByGame(gameID string) (*model.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.jobsByGame[gameID]
	if !ok {
		return nil, false
	}
	j, ok := s.jobs[id]
	return j, ok
}

func (s *MemoryStore) CreateGroup(gr *model.Group) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups[gr.ID] = gr
}

func (s *MemoryStore) GetGroup(id string) (*model.Group, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	gr, ok := s.groups[id]
	return gr, ok
}

func (s *MemoryStore) UpdateGroup(gr *model.Group) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups[gr.ID] = gr
}

func (s *MemoryStore) GetGroupByInvite(code string) (*model.Group, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, gr := range s.groups {
		if gr.InviteCode == code {
			return gr, true
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
