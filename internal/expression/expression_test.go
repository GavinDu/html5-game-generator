package expression

import (
	"testing"

	"doodlequest/internal/gameir"
)

func draft() *Document {
	return &Document{
		SchemaVersion: SchemaVersion,
		Status:        StatusDraft,
		Source:        Source{Type: "image", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Creator:       Creator{OwnerID: "creator-1", Consent: true, Visibility: VisibilityPrivate},
		Mode:          ModeWhatIf,
	}
}

func TestDraftRequiresProvenanceAndConsent(t *testing.T) {
	doc := draft()
	if err := doc.Validate(); err != nil {
		t.Fatal(err)
	}
	doc.Creator.Consent = false
	if err := doc.Validate(); err == nil {
		t.Fatal("expected consent failure")
	}
}

func TestReadyRequiresTraceableInterpretation(t *testing.T) {
	doc := draft()
	doc.Status = StatusReady
	doc.Intent = "A quiet world"
	doc.ChosenInterpretation = "A moon garden"
	doc.Anchors = []Anchor{{ID: "a1", Kind: "place", Value: "garden", Preserve: true}}
	doc.Evidence = []Evidence{{ID: "e1", Kind: "image_region", Start: 0, End: 1}}
	doc.Interpretation.Preserved = []Claim{{ID: "p1", Text: "garden", Confidence: .9, EvidenceIDs: []string{"e1"}}}
	doc.Generation = Generation{GameIRVersion: "1.2", PackageVersion: "2.0"}
	if err := doc.Validate(); err != nil {
		t.Fatal(err)
	}
	doc.Interpretation.Preserved[0].EvidenceIDs = []string{"missing"}
	if err := doc.Validate(); err == nil {
		t.Fatal("expected unknown evidence failure")
	}
}

func TestRemixRequiresParentAndModeIsAllowlisted(t *testing.T) {
	doc := draft()
	doc.Mode = "other"
	if err := doc.Validate(); err == nil {
		t.Fatal("expected invalid mode")
	}
	doc.Mode = ModeRemix
	if err := doc.Validate(); err == nil {
		t.Fatal("expected missing parent")
	}
}

func TestValidateAgainstGameIRRequiresPreservedAnchorsAndMatchingVersion(t *testing.T) {
	doc := &Document{
		SchemaVersion:        SchemaVersion,
		Status:               StatusReady,
		Source:               Source{Type: "text", Reference: "source-1"},
		Creator:              Creator{OwnerID: "creator-1", Consent: true, Visibility: VisibilityPrivate},
		Mode:                 ModeMakePlayable,
		Intent:               "A garden adventure",
		Anchors:              []Anchor{{ID: "a1", Kind: "place", Value: "moon garden", Preserve: true}},
		ChosenInterpretation: "Explore a moon garden",
		Interpretation:       Interpretation{Preserved: []Claim{{ID: "p1", Text: "moon garden", Confidence: .9}}},
		Generation:           Generation{GameIRVersion: "1.2", PackageVersion: "2.0"},
	}
	ir := &gameir.Document{IRVersion: "1.2", Game: gameir.Game{ID: "g1", Title: "Garden Adventure", Description: "A moon garden adventure"}}
	if err := ValidateAgainstGameIR(doc, ir); err != nil {
		t.Fatalf("expected preserved anchor to validate: %v", err)
	}
	ir.Game.Description = "A quiet adventure"
	if err := ValidateAgainstGameIR(doc, ir); err == nil {
		t.Fatal("missing preserved anchor must block publication")
	}
	ir.Game.Description = "A moon garden adventure"
	ir.IRVersion = "1.1"
	if err := ValidateAgainstGameIR(doc, ir); err == nil {
		t.Fatal("mismatched Game IR version must block publication")
	}
}
