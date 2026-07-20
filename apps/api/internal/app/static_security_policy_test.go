package app

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var explainedNosecPattern = regexp.MustCompile(`#nosec G[0-9]{3}(,G[0-9]{3})* -- \S.+`)

func TestStaticSecuritySuppressionsAreRuleSpecificAndExplained(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve static security policy test path")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	err := filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || path == currentFile {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		scanner := bufio.NewScanner(file)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := scanner.Text()
			if strings.Contains(line, "#nosec") && !explainedNosecPattern.MatchString(line) {
				t.Errorf("%s:%d: security suppression must name exact G-rules and include an explanation", path, lineNumber)
			}
		}
		return scanner.Err()
	})
	if err != nil {
		t.Fatalf("scan static security suppressions: %v", err)
	}
}
