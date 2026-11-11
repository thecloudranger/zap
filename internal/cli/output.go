package cli

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/thecloudranger/zap/internal/gitx"
)

// SchemaVersion is bumped on any breaking change to the --json envelope or its data.
const SchemaVersion = 1

// Envelope is the single JSON document written to stdout by every --json command.
type Envelope struct {
	SchemaVersion int        `json:"schemaVersion"`
	Command       string     `json:"command"`
	Status        string     `json:"status"` // "ok" or "error"
	Data          any        `json:"data,omitempty"`
	Warnings      []string   `json:"warnings"`
	Error         *ErrorBody `json:"error,omitempty"`
}

// ErrorBody is a machine-readable error. Code values are stable.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w io.Writer, env Envelope) error {
	env.SchemaVersion = SchemaVersion
	if env.Warnings == nil {
		env.Warnings = []string{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}

func writeOK(w io.Writer, command string, data any) error {
	return writeJSON(w, Envelope{Command: command, Status: "ok", Data: data})
}

// writeErr writes an error envelope and returns an already-reported ExitError.
func writeErr(w io.Writer, command string, err error) error {
	body := &ErrorBody{Code: errorCode(err), Message: err.Error()}
	if werr := writeJSON(w, Envelope{Command: command, Status: "error", Error: body}); werr != nil {
		return errors.Join(err, werr)
	}
	return &ExitError{Code: ExitFailure, Err: err, Reported: true}
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, gitx.ErrNotRepo):
		return "not_a_repo"
	case errors.Is(err, gitx.ErrNoTarget):
		return "no_target"
	case errors.Is(err, ErrNotWorktree):
		return "not_a_worktree"
	default:
		return "error"
	}
}
