package gameir

import (
	"os"
	"strings"
	"testing"
)

func readFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../examples/game-ir-v1/bobos-rainbow.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGoldenFixtureIsValid(t *testing.T) {
	if _, err := DecodeAndValidate(readFixture(t)); err != nil {
		t.Fatalf("golden fixture should validate: %v", err)
	}
}

func TestRejectsUnknownExecutableField(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"irVersion": "1.0"`, `"irVersion": "1.0", "executableScript": "print(1)"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected unknown script field to be rejected")
	}
}

func TestRejectsPathTraversal(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"uri": "assets/field.png"`, `"uri": "assets/../secret.png"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestRejectsMissingAssetReference(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"assetId": "bobo"`, `"assetId": "missing"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected missing asset reference to be rejected")
	}
}

func TestRejectsUnsupportedAction(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"type": "add_score"`, `"type": "execute_script"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected unsupported action to be rejected")
	}
}

func TestRejectsTrailingJSON(t *testing.T) {
	data := append(readFixture(t), []byte("{}")...)
	if _, err := DecodeAndValidate(data); err == nil {
		t.Fatal("expected trailing JSON value to be rejected")
	}
}

func TestRejectsUnsupportedVersion(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"irVersion": "1.0"`, `"irVersion": "2.0"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected unsupported version to be rejected")
	}
}

func TestRejectsDuplicateEntityIDs(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"id": "kitten",
          "kind": "decoration",
          "assetId": "kitten"`, `"id": "player",
          "kind": "decoration",
          "assetId": "kitten"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected duplicate entity ID to be rejected")
	}
}
