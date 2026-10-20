// Package model holds the domain types for the DoodleQuest backend:
// games generated from kids' drawings.
package model

import (
	"time"

	"doodlequest/internal/gameir"
)

// Visibility controls who can see/play a game. The author owns it.
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityGroups  Visibility = "groups"
	VisibilityPublic  Visibility = "public"
)

// Element is one thing the vision stage pulled out of a kid's drawing:
// a character, prop, or background piece.
type Element struct {
	ID          string      `json:"id"`
	Label       string      `json:"label"`
	Kind        string      `json:"kind"` // character | prop | background
	Box         *[4]float64 `json:"box,omitempty"`
	Description string      `json:"description"`
}

type ImageInput struct {
	URL     string `json:"url,omitempty"`
	DataURL string `json:"data_url,omitempty"`
}

type IngestRequest struct {
	Images   []ImageInput `json:"images"`
	Keywords []string     `json:"keywords"`
}

type IngestResult struct {
	IngestID      string    `json:"ingest_id"`
	Elements      []Element `json:"elements"`
	GuessedIntent string    `json:"guessed_intent"`
	Keywords      []string  `json:"keywords"`
	CreatedAt     time.Time `json:"created_at"`
}

// DesignDoc is the structured game design the user confirms before codegen.
type DesignDoc struct {
	Title         string    `json:"title"`
	Plot          string    `json:"plot"`
	Mechanics     string    `json:"mechanics"`
	Controls      []string  `json:"controls"`
	WinCondition  string    `json:"win_condition"`
	LoseCondition string    `json:"lose_condition"`
	Elements      []Element `json:"elements"`
	MusicMood     string    `json:"music_mood"`
}

type StageName string

const (
	StageSpec     StageName = "spec"
	StageArt      StageName = "art"
	StageCodegen  StageName = "codegen"
	StageValidate StageName = "validate"
	StagePublish  StageName = "publish"
)

type StageStatus string

const (
	StagePending StageStatus = "pending"
	StageRunning StageStatus = "running"
	StageDone    StageStatus = "done"
	StageFailed  StageStatus = "failed"
)

// StageRecord is the per-stage cost/quality instrumentation. Every stage
// records latency, tokens, and cost from day one — even stub stages.
type StageRecord struct {
	Name      StageName   `json:"name"`
	Status    StageStatus `json:"status"`
	LatencyMs int64       `json:"latency_ms"`
	TokensIn  int         `json:"tokens_in"`
	TokensOut int         `json:"tokens_out"`
	CostUSD   float64     `json:"cost_usd"`
	Provider  string      `json:"provider"`
	Error     string      `json:"error,omitempty"`
	Quality   *float64    `json:"quality_score,omitempty"`
	StartedAt *time.Time  `json:"started_at,omitempty"`
	EndedAt   *time.Time  `json:"ended_at,omitempty"`
}

type JobStatus string

const (
	JobQueued          JobStatus = "queued"
	JobRunning         JobStatus = "running"
	JobAwaitingConfirm JobStatus = "awaiting_confirm"
	JobDone            JobStatus = "done"
	JobFailed          JobStatus = "failed"
)

type Job struct {
	ID        string        `json:"job_id"`
	GameID    string        `json:"game_id"`
	Status    JobStatus     `json:"status"`
	Stages    []StageRecord `json:"stages"`
	DesignDoc *DesignDoc    `json:"design_doc,omitempty"`
	Error     string        `json:"error,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type GameStatus string

const (
	GameDraft      GameStatus = "draft"
	GameGenerating GameStatus = "generating"
	GameReady      GameStatus = "ready"
	GameFailed     GameStatus = "failed"
)

type SpriteAsset struct {
	ID     string `json:"id"`
	Src    string `json:"src"`
	Source string `json:"source"` // kid-drawing | procedural
	Label  string `json:"label"`
}

type MusicAsset struct {
	Src    string `json:"src"`
	Mood   string `json:"mood"`
	Source string `json:"source"` // library | generated
}

type BundleManifest struct {
	GameID   string   `json:"game_id"`
	Title    string   `json:"title"`
	Version  int      `json:"version"`
	Shim     string   `json:"shim"`
	Entry    string   `json:"entry"`
	Controls []string `json:"controls"`
	Assets   struct {
		Sprites []SpriteAsset `json:"sprites"`
		Music   MusicAsset    `json:"music"`
	} `json:"assets"`
	Visibility     Visibility `json:"visibility"`
	SharedGroupIDs []string   `json:"shared_group_ids"`
}

type Game struct {
	ID         string          `json:"game_id"`
	Title      string          `json:"title"`
	OwnerID    string          `json:"owner_id"`
	Elements   []Element       `json:"elements"`
	Intent     string          `json:"intent"`
	Visibility Visibility      `json:"visibility"`
	GroupIDs   []string        `json:"group_ids"`
	Status     GameStatus      `json:"status"`
	JobID      string          `json:"job_id"`
	Manifest   *BundleManifest `json:"manifest,omitempty"`
	GameIR     *gameir.Document `json:"game_ir,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type Group struct {
	ID         string    `json:"group_id"`
	Name       string    `json:"name"`
	OwnerID    string    `json:"owner_id"`
	InviteCode string    `json:"invite_code"`
	MemberIDs  []string  `json:"member_ids"`
	CreatedAt  time.Time `json:"created_at"`
}

// ScoreEntry is one leaderboard row: a player's best run in a game,
// ranked by level reached first, then score.
type ScoreEntry struct {
	Name      string    `json:"name"`
	Score     int       `json:"score"`
	Level     int       `json:"level"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateGameRequest struct {
	IngestID string    `json:"ingest_id"`
	Elements []Element `json:"elements"`
	Intent   string    `json:"intent"`
	Keywords []string  `json:"keywords"`
}

type ConfirmRequest struct {
	DesignDoc *DesignDoc `json:"design_doc,omitempty"`
}

type VisibilityRequest struct {
	Visibility Visibility `json:"visibility"`
	GroupIDs   []string   `json:"group_ids"`
}

type JoinGroupRequest struct {
	InviteCode string `json:"invite_code"`
}

type CreateGroupRequest struct {
	Name string `json:"name"`
}
