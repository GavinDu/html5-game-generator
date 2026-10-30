// Package expression defines the versioned source-expression contract.
// It is provenance metadata, not a second runtime format.
package expression

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const SchemaVersion = "1.0"

type Status string

const (
	StatusDraft Status = "draft"
	StatusReady Status = "ready"
)

type Mode string

const (
	ModeMakePlayable Mode = "make_playable"
	ModeWhatIf       Mode = "what_if"
	ModeRemix        Mode = "remix"
)

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityGroups  Visibility = "groups"
	VisibilityPublic  Visibility = "public"
)

type Source struct {
	Type      string `json:"type"`
	Reference string `json:"reference,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

type Creator struct {
	OwnerID    string     `json:"ownerId"`
	Consent    bool       `json:"consent"`
	Visibility Visibility `json:"visibility"`
}

type Anchor struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Value    string `json:"value"`
	Preserve bool   `json:"preserve"`
}

type Evidence struct {
	ID              string  `json:"id"`
	Kind            string  `json:"kind"`
	SourceReference string  `json:"sourceReference,omitempty"`
	Start           float64 `json:"start,omitempty"`
	End             float64 `json:"end,omitempty"`
	Quote           string  `json:"quote,omitempty"`
}

type Claim struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Confidence  float64  `json:"confidence"`
	EvidenceIDs []string `json:"evidenceIds,omitempty"`
}

type Interpretation struct {
	Preserved []Claim `json:"preserved,omitempty"`
	Invented  []Claim `json:"invented,omitempty"`
	Uncertain []Claim `json:"uncertain,omitempty"`
}

type Generation struct {
	Seed           int64  `json:"seed"`
	GameIRVersion  string `json:"gameIrVersion,omitempty"`
	PackageVersion string `json:"packageVersion,omitempty"`
	ModelVersion   string `json:"modelVersion,omitempty"`
	PromptVersion  string `json:"promptVersion,omitempty"`
}

type Lineage struct {
	ParentGameID       string `json:"parentGameId,omitempty"`
	ParentVersion      int    `json:"parentVersion,omitempty"`
	ParentExpressionID string `json:"parentExpressionId,omitempty"`
}

type Document struct {
	SchemaVersion       string         `json:"schemaVersion"`
	Status              Status         `json:"status"`
	Source              Source         `json:"source"`
	Creator             Creator        `json:"creator"`
	Mode                Mode           `json:"mode"`
	Intent              string         `json:"intent,omitempty"`
	Anchors             []Anchor       `json:"anchors,omitempty"`
	Motifs              []string       `json:"motifs,omitempty"`
	OptionalDetails     []string       `json:"optionalDetails,omitempty"`
	Evidence            []Evidence     `json:"evidence,omitempty"`
	InteractiveVerbs    []string       `json:"interactiveVerbs,omitempty"`
	CandidateMechanics  []string       `json:"candidateMechanics,omitempty"`
	ChosenInterpretation string        `json:"chosenInterpretation,omitempty"`
	Interpretation      Interpretation `json:"interpretation,omitempty"`
	Generation          Generation     `json:"generation"`
	Lineage             Lineage         `json:"lineage,omitempty"`
}

func (d *Document) Validate() error {
	if d == nil {
		return fmt.Errorf("playable expression is required")
	}
	if d.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported playable expression schema version %q", d.SchemaVersion)
	}
	if d.Status != StatusDraft && d.Status != StatusReady {
		return fmt.Errorf("status must be draft or ready")
	}
	switch d.Source.Type {
	case "text", "image", "drawing", "audio", "video":
	default:
		return fmt.Errorf("unsupported source type %q", d.Source.Type)
	}
	if strings.TrimSpace(d.Source.Reference) == "" && strings.TrimSpace(d.Source.SHA256) == "" {
		return fmt.Errorf("source reference or sha256 is required")
	}
	if d.Source.SHA256 != "" {
		decoded, err := hex.DecodeString(d.Source.SHA256)
		if err != nil || len(decoded) != 32 {
			return fmt.Errorf("source sha256 must be a 64-character hexadecimal digest")
		}
	}
	if strings.TrimSpace(d.Creator.OwnerID) == "" || !d.Creator.Consent {
		return fmt.Errorf("creator ownerId and consent are required")
	}
	switch d.Creator.Visibility {
	case VisibilityPrivate, VisibilityGroups, VisibilityPublic:
	default:
		return fmt.Errorf("unsupported creator visibility %q", d.Creator.Visibility)
	}
	switch d.Mode {
	case ModeMakePlayable, ModeWhatIf, ModeRemix:
	default:
		return fmt.Errorf("unsupported mode %q", d.Mode)
	}

	anchors := make(map[string]bool)
	for _, anchor := range d.Anchors {
		if strings.TrimSpace(anchor.ID) == "" || strings.TrimSpace(anchor.Value) == "" {
			return fmt.Errorf("anchor id and value are required")
		}
		if anchors[anchor.ID] {
			return fmt.Errorf("duplicate anchor id %q", anchor.ID)
		}
		anchors[anchor.ID] = true
		switch anchor.Kind {
		case "character", "object", "place", "word", "event", "feeling":
		default:
			return fmt.Errorf("unsupported anchor kind %q", anchor.Kind)
		}
	}

	evidence := make(map[string]bool)
	for _, item := range d.Evidence {
		if strings.TrimSpace(item.ID) == "" || evidence[item.ID] {
			return fmt.Errorf("evidence id is empty or duplicated: %q", item.ID)
		}
		evidence[item.ID] = true
		switch item.Kind {
		case "text_span", "image_region", "video_timestamp", "audio_timestamp":
		default:
			return fmt.Errorf("unsupported evidence kind %q", item.Kind)
		}
		if item.Start < 0 || item.End < item.Start {
			return fmt.Errorf("invalid evidence range %q", item.ID)
		}
	}

	claimGroups := map[string][]Claim{
		"preserved": d.Interpretation.Preserved,
		"invented":  d.Interpretation.Invented,
		"uncertain": d.Interpretation.Uncertain,
	}
	for label, claims := range claimGroups {
		for _, claim := range claims {
			if strings.TrimSpace(claim.ID) == "" || strings.TrimSpace(claim.Text) == "" ||
				claim.Confidence < 0 || claim.Confidence > 1 {
				return fmt.Errorf("invalid %s claim %q", label, claim.ID)
			}
			for _, evidenceID := range claim.EvidenceIDs {
				if !evidence[evidenceID] {
					return fmt.Errorf("%s claim %q references unknown evidence %q", label, claim.ID, evidenceID)
				}
			}
		}
	}

	if d.Status == StatusReady {
		if strings.TrimSpace(d.Intent) == "" || len(d.Anchors) == 0 ||
			strings.TrimSpace(d.ChosenInterpretation) == "" {
			return fmt.Errorf("ready expression requires intent, anchors, and chosenInterpretation")
		}
		if len(d.Interpretation.Preserved)+len(d.Interpretation.Invented)+len(d.Interpretation.Uncertain) == 0 {
			return fmt.Errorf("ready expression requires interpretation claims")
		}
		if strings.TrimSpace(d.Generation.GameIRVersion) == "" ||
			strings.TrimSpace(d.Generation.PackageVersion) == "" {
			return fmt.Errorf("ready expression requires Game IR and package versions")
		}
	}
	if d.Lineage.ParentVersion < 0 {
		return fmt.Errorf("parentVersion cannot be negative")
	}
	if d.Mode == ModeRemix && strings.TrimSpace(d.Lineage.ParentGameID) == "" {
		return fmt.Errorf("remix mode requires parentGameId")
	}
	return nil
}
