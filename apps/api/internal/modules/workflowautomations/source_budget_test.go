package workflowautomations

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowAutomationSourceSizeRatchet(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read workflow automation package: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		const maximum = 500
		source, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		lines := bytes.Count(source, []byte{'\n'})
		if len(source) > 0 && source[len(source)-1] != '\n' {
			lines++
		}
		if lines > maximum {
			t.Errorf("%s has %d lines, exceeding the workflow module's %d-line no-growth budget; split a tested definition, execution, or evidence seam before adding behavior", name, lines, maximum)
		}
	}
}
