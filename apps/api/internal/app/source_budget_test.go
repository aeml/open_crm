package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationSourceSizeRatchet(t *testing.T) {
	maximums := map[string]int{
		"app.go":              1000,
		"support_handlers.go": 800,
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read application package: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		maximum := 500
		if configured, ok := maximums[name]; ok {
			maximum = configured
		}
		source, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		lines := bytes.Count(source, []byte{'\n'})
		if len(source) > 0 && source[len(source)-1] != '\n' {
			lines++
		}
		if lines > maximum {
			t.Errorf("%s has %d lines, exceeding its %d-line no-growth budget; split a tested domain seam or deliberately lower nearby code before adding more", name, lines, maximum)
		}
	}
}
