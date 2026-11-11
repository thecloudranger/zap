// Package oplog is zap's append-only operations log: one JSON object per line.
package oplog

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileName is the log file inside the state directory.
const FileName = "operations.jsonl"

// Entry records one attempted removal.
type Entry struct {
	Run           string    `json:"run"` // groups entries of one zap clean
	Time          time.Time `json:"time"`
	Repo          string    `json:"repo"`
	Path          string    `json:"path"`
	Branch        string    `json:"branch,omitempty"`
	Head          string    `json:"head"`
	Status        string    `json:"status"`
	BackupRef     string    `json:"backupRef,omitempty"`
	BranchDeleted bool      `json:"branchDeleted"`
	Result        string    `json:"result"` // "zapped", "skipped" or "failed"
	Detail        string    `json:"detail,omitempty"`
}

// Log appends to and reads <dir>/operations.jsonl.
type Log struct {
	Dir string
}

// Append writes entries, creating the directory if needed.
func (l Log) Append(entries ...Entry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := os.MkdirAll(l.Dir, 0o755); err != nil {
		return fmt.Errorf("oplog: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(l.Dir, FileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("oplog: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			_ = f.Close()
			return fmt.Errorf("oplog: write: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("oplog: %w", err)
	}
	return nil
}

// Read returns all entries, newest first. A missing log is empty. Lines that do not
// parse (for example a torn final write) are skipped and counted.
func (l Log) Read() (entries []Entry, skipped int, err error) {
	f, err := os.Open(filepath.Join(l.Dir, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("oplog: %w", err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			skipped++
			continue
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, skipped, fmt.Errorf("oplog: read: %w", err)
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, skipped, nil
}
