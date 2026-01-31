package synthesizer

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrNeedsHumanReview = errors.New("NEEDS_HUMAN_REVIEW")
)

type Synthesizer struct{}

func New() *Synthesizer {
	return &Synthesizer{}
}

// FillBlocks replaces AI markers with code based on the intent (service name for mocking).
func (s *Synthesizer) FillBlocks(filename, content, intent string) (string, error) {
	if strings.Contains(intent, "ambiguous") {
		return "", ErrNeedsHumanReview
	}

	shouldFail := strings.Contains(intent, "fail")

	// Regex for logic blocks
	// // AI_LOGIC_START: Name
	// ...
	// // AI_LOGIC_END: Name
	reLogic := regexp.MustCompile(`(?s)// AI_LOGIC_START: (\w+).*?// AI_LOGIC_END: \w+`)

	content = reLogic.ReplaceAllStringFunc(content, func(match string) string {
		submatches := reLogic.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		funcName := submatches[1]

		if shouldFail {
			return fmt.Sprintf(`w.Write("Syntax Error Here %s)`, funcName) // Missing quote
		}
		return fmt.Sprintf(`w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello from %s"))`, funcName)
	})

	// Regex for test blocks
	reTest := regexp.MustCompile(`(?s)// AI_TEST_START: (\w+).*?// AI_TEST_END: \w+`)
	content = reTest.ReplaceAllStringFunc(content, func(match string) string {
		submatches := reTest.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		funcName := submatches[1]

		if shouldFail {
			return `if true { t.Fail() } syntax error`
		}
		return fmt.Sprintf(`req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	%s(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("Expected 200")
	}`, funcName)
	})

	return content, nil
}
