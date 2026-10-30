package expression

import "testing"

func draft() *Document {
	return &Document{
		SchemaVersion: SchemaVersion,
		Status:  StatusDraft,
		Source:  Source{Type: "image", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Creator: Creator{OwnerID: "creator-1", Consent: true, Visibility: VisibilityPrivate},
		Mode:    ModeWhatIf,
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
