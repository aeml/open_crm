package telemetry

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxStatusBytes = 64 * 1024

type BackupStatus struct {
	Available                   bool
	LastSuccessAt               time.Time
	LastAttemptSucceeded        bool
	LastRestoreSuccessAt        time.Time
	LastRestoreAttemptSucceeded bool
}

type statusDocument struct {
	Status      string `json:"status"`
	CompletedAt string `json:"completedAt"`
}

// ReadBackupStatus reads only bounded, non-customer operational evidence. Any
// missing or malformed required file leaves Available false so alert rules fail
// closed instead of treating corrupt evidence as a healthy backup.
func ReadBackupStatus(directory string) BackupStatus {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return BackupStatus{}
	}
	lastBackup, backupErr := readStatusFile(filepath.Join(directory, "last-backup.json"))
	lastBackupAttempt, backupAttemptErr := readStatusFile(filepath.Join(directory, "last-backup-attempt.json"))
	lastRestore, restoreErr := readStatusFile(filepath.Join(directory, "last-restore-drill.json"))
	lastRestoreAttempt, restoreAttemptErr := readStatusFile(filepath.Join(directory, "last-restore-drill-attempt.json"))
	lastBackupAt := parseStatusTime(lastBackup.CompletedAt)
	lastRestoreAt := parseStatusTime(lastRestore.CompletedAt)
	if backupErr != nil || backupAttemptErr != nil || restoreErr != nil || restoreAttemptErr != nil ||
		lastBackup.Status != "succeeded" || lastRestore.Status != "succeeded" || lastBackupAt.IsZero() || lastRestoreAt.IsZero() {
		return BackupStatus{}
	}
	return BackupStatus{
		Available:                   true,
		LastSuccessAt:               lastBackupAt,
		LastAttemptSucceeded:        lastBackupAttempt.Status == "succeeded",
		LastRestoreSuccessAt:        lastRestoreAt,
		LastRestoreAttemptSucceeded: lastRestoreAttempt.Status == "succeeded",
	}
}

func readStatusFile(path string) (statusDocument, error) {
	file, err := os.Open(path)
	if err != nil {
		return statusDocument{}, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, maxStatusBytes))
	var document statusDocument
	if err := decoder.Decode(&document); err != nil {
		return statusDocument{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("backup status contains trailing data")
		}
		return statusDocument{}, err
	}
	document.Status = strings.TrimSpace(strings.ToLower(document.Status))
	return document, nil
}

func parseStatusTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}
