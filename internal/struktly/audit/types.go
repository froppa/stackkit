package audit

import (
	"time"
)

type AuditLog struct {
	Timestamp    time.Time          `json:"timestamp"`
	Inputs       InputHashes        `json:"inputs"`
	Policy       PolicyResult       `json:"policy"`
	Generation   GenerationResult   `json:"generation"`
	Verification VerificationResult `json:"verification"`
	Outcome      string             `json:"outcome"` // SUCCESS, FAIL, REVIEW
}

type InputHashes struct {
	SpecHash   string `json:"spec_hash"`
	PolicyHash string `json:"policy_hash"`
}

type PolicyResult struct {
	Passed bool   `json:"passed"`
	Reason string `json:"reason,omitempty"`
}

type GenerationResult struct {
	Prompts []string `json:"prompts"`
	Model   string   `json:"model"`
}

type VerificationResult struct {
	Passed bool   `json:"passed"`
	Output string `json:"output,omitempty"`
}
