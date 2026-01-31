package gates

import (
	"bytes"
	"fmt"
	"os/exec"
)

type Runner struct{}

func New() *Runner {
	return &Runner{}
}

// Verify runs go mod init, go fmt, go vet, and go test.
// Returns the combined output and an error if any step fails.
func (r *Runner) Verify(dir string) (string, error) {
	var combinedOutput bytes.Buffer

	// Helper to run and log
	run := func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		var out bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr

		combinedOutput.WriteString(fmt.Sprintf("Running: %s %v\n", name, args))
		err := cmd.Run()

		combinedOutput.Write(out.Bytes())
		combinedOutput.Write(stderr.Bytes())
		combinedOutput.WriteString("\n")

		if err != nil {
			return fmt.Errorf("%s failed: %w", name, err)
		}
		return nil
	}

	// 1. Init Module
	if err := run("go", "mod", "init", "generated"); err != nil {
		return combinedOutput.String(), err
	}

	// 2. Fmt
	if err := run("go", "fmt", "./..."); err != nil {
		return combinedOutput.String(), err
	}

	// 3. Vet
	if err := run("go", "vet", "./..."); err != nil {
		return combinedOutput.String(), err
	}

	// 4. Test
	if err := run("go", "test", "./..."); err != nil {
		return combinedOutput.String(), err
	}

	return combinedOutput.String(), nil
}
