package configkit_test

import (
	"testing"

	config "github.com/froppa/stackkit/kits/configkit"
)

func TestRedactNested(t *testing.T) {
	raw := map[string]any{
		"database": map[string]any{
			"user":     "svc",
			"password": "secret",
		},
		"api": map[string]any{
			"token": "abc",
		},
	}

	got := config.Redact("", raw).(map[string]any)
	db := got["database"].(map[string]any)
	if db["password"] != "***" {
		t.Fatalf("expected password redacted, got %v", db["password"])
	}
	api := got["api"].(map[string]any)
	if api["token"] != "***" {
		t.Fatalf("expected token redacted, got %v", api["token"])
	}
}
