package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Exit codes are part of zap's public contract; never renumber them.
const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitRefused     = 4 // zap refused to act (disclaimer not accepted, confirmation missing)
	ExitInterrupted = 130
)

// ExitError carries an exit code. Reported means the error was already written
// (for example inside a --json envelope) and must not be printed again.
type ExitError struct {
	Code     int
	Err      error
	Reported bool
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// Exit converts the error returned by the root command into a process exit code,
// printing it to stderr unless it was already reported.
func Exit(err error, stderr io.Writer) int {
	if err == nil {
		return ExitOK
	}
	code := ExitFailure
	var ee *ExitError
	if errors.As(err, &ee) {
		code = ee.Code
		if ee.Reported {
			return code
		}
	} else if errors.Is(err, context.Canceled) {
		code = ExitInterrupted
	}
	_, _ = fmt.Fprintf(stderr, "zap: %v\n", err)
	return code
}

func usageErr(err error) error { return &ExitError{Code: ExitUsage, Err: err} }

// usageArgs marks positional-argument validation failures as usage errors.
func usageArgs(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := v(cmd, args); err != nil {
			return usageErr(err)
		}
		return nil
	}
}
