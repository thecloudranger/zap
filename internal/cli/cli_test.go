package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExit(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		code      int
		printsErr bool
	}{
		{"nil", nil, ExitOK, false},
		{"plain", errors.New("boom"), ExitFailure, true},
		{"usage", usageErr(errors.New("bad flag")), ExitUsage, true},
		{"reported", &ExitError{Code: ExitFailure, Err: errors.New("x"), Reported: true}, ExitFailure, false},
		{"canceled", fmt.Errorf("scan: %w", context.Canceled), ExitInterrupted, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			assert.Equal(t, tt.code, Exit(tt.err, &buf))
			assert.Equal(t, tt.printsErr, buf.Len() > 0)
		})
	}
}

func TestTildifyAndRel(t *testing.T) {
	assert.Equal(t, "~/dev/x", tildify("/Users/u/dev/x", "/Users/u"))
	assert.Equal(t, "/Users/other", tildify("/Users/other", "/Users/u"))
	assert.Equal(t, "/Users/uu/x", tildify("/Users/uu/x", "/Users/u"), "prefix must end at a separator")
	assert.Equal(t, ".claude/worktrees/a", relTo("/r/.claude/worktrees/a", "/r", "/Users/u"))
	assert.Equal(t, "~/.codex/worktrees/1", relTo("/Users/u/.codex/worktrees/1", "/r", "/Users/u"))
}
