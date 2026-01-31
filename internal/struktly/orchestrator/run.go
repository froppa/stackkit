package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/froppa/stackkit/internal/struktly/audit"
	"github.com/froppa/stackkit/internal/struktly/gates"
	"github.com/froppa/stackkit/internal/struktly/generator"
	"github.com/froppa/stackkit/internal/struktly/ir"
	"github.com/froppa/stackkit/internal/struktly/policy"
	"github.com/froppa/stackkit/internal/struktly/synthesizer"
)

func Run(specPath, policyPath, outDir string) error {
	log := audit.AuditLog{
		Timestamp: time.Now(),
		Outcome:   "FAIL",
	}

	// Ensure outDir exists
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	defer func() {
		// Write Audit Artifacts
		writeJSON(outDir, "audit.json", log)
		writeJSON(outDir, "policy.json", log.Policy)
		writeJSON(outDir, "verification.json", log.Verification)
		writeJSON(outDir, "generation.json", log.Generation)
	}()

	// 1. Hash Inputs
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("read spec failed: %w", err)
	}
	policyBytes, err := os.ReadFile(policyPath)
	if err != nil {
		return fmt.Errorf("read policy failed: %w", err)
	}
	log.Inputs.SpecHash = hash(specBytes)
	log.Inputs.PolicyHash = hash(policyBytes)

	// 2. Parse & Policy
	svc, err := ir.Parse(specPath)
	if err != nil {
		log.Policy.Reason = "Parse Error: " + err.Error()
		return err
	}

	pol, err := policy.Load(policyPath)
	if err != nil {
		log.Policy.Reason = "Policy Load Error: " + err.Error()
		return err
	}

	if err := policy.Check(pol, svc); err != nil {
		log.Policy.Passed = false
		log.Policy.Reason = err.Error()
		return err
	}
	log.Policy.Passed = true

	// 3. Generate Skeleton
	files, err := generator.Render(svc)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	// 4. Synthesize
	synth := synthesizer.New()
	for name, content := range files {
		newContent, err := synth.FillBlocks(name, content, svc.Name) // Use svc.Name as intent
		if err != nil {
			if err == synthesizer.ErrNeedsHumanReview {
				log.Outcome = "NEEDS_HUMAN_REVIEW"
				return err
			}
			return fmt.Errorf("synthesis failed: %w", err)
		}
		files[name] = newContent
	}
	log.Generation.Model = "mock-v1"

	// 5. Write Temp
	tmpDir, err := os.MkdirTemp("", "struktly-*")
	if err != nil {
		return fmt.Errorf("temp dir failed: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			return fmt.Errorf("write temp file failed: %w", err)
		}
	}

	// 6. Verify
	gateRunner := gates.New()
	output, err := gateRunner.Verify(tmpDir)
	log.Verification.Output = output
	if err != nil {
		log.Verification.Passed = false
		log.Outcome = "FAIL"
		return fmt.Errorf("verification failed: %w", err)
	}
	log.Verification.Passed = true
	log.Outcome = "SUCCESS"

	// 7. Copy to Output
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(outDir, name), []byte(content), 0644); err != nil {
			return fmt.Errorf("write output file failed: %w", err)
		}
	}

	return nil
}

func hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func writeJSON(dir, filename string, v interface{}) {
	f, err := os.Create(filepath.Join(dir, filename))
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
