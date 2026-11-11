// Command zap finds and removes stale git worktrees.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/thecloudranger/zap/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := cli.NewRoot(cli.DefaultDeps()).ExecuteContext(ctx)
	stop()
	os.Exit(cli.Exit(err, os.Stderr))
}
