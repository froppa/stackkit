package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestAuditLog_JSONFileRoundTrip writes an AuditLog the way orchestrator.writeJSON
// does and checks the file content and that it decodes back to the same value.
func TestAuditLog_JSONFileRoundTrip(t *testing.T) {
	want := AuditLog{
		Timestamp: time.Date(2026, 9, 21, 12, 30, 45, 123456789, time.UTC),
		Inputs: InputHashes{
			SpecHash:   "spec-sha256",
			PolicyHash: "policy-sha256",
		},
		Policy: PolicyResult{Passed: true, Reason: "within limits"},
		Generation: GenerationResult{
			Prompts: []string{"first prompt", "second prompt"},
			Model:   "test-model",
		},
		Verification: VerificationResult{Passed: true, Output: "ok"},
		Outcome:      "SUCCESS",
	}

	path := filepath.Join(t.TempDir(), "audit.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(want); err != nil {
		_ = f.Close()
		t.Fatalf("encode: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := string(data)
	for _, fragment := range []string{
		`"spec_hash": "spec-sha256"`,
		`"policy_hash": "policy-sha256"`,
		"\"prompts\": [\n      \"first prompt\",\n      \"second prompt\"\n    ]",
		`"model": "test-model"`,
		`"outcome": "SUCCESS"`,
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("file content missing %q:\n%s", fragment, content)
		}
	}

	var got AuditLog
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, want.Timestamp)
	}
	got.Timestamp = want.Timestamp
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decoded AuditLog = %+v, want %+v", got, want)
	}
}
