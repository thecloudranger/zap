package gitx

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thecloudranger/zap/internal/model"
)

// z joins porcelain attributes the way `--porcelain -z` emits them.
func z(records ...[]string) []byte {
	var b strings.Builder
	for _, rec := range records {
		for _, attr := range rec {
			b.WriteString(attr)
			b.WriteByte(0)
		}
		b.WriteByte(0)
	}
	return []byte(b.String())
}

func TestParseWorktreeList(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []model.Worktree
	}{
		{
			name: "main only",
			in:   z([]string{"worktree /r", "HEAD aaa", "branch refs/heads/main"}),
			want: []model.Worktree{{Path: "/r", Head: "aaa", Branch: "main", Main: true}},
		},
		{
			name: "linked, detached, locked with reason, prunable",
			in: z(
				[]string{"worktree /r", "HEAD aaa", "branch refs/heads/main"},
				[]string{"worktree /r/.claude/worktrees/x", "HEAD bbb", "branch refs/heads/worktree-x", "locked claude agent agent-1 (pid 42)"},
				[]string{"worktree /codex/1", "HEAD ccc", "detached"},
				[]string{"worktree /gone", "HEAD ddd", "branch refs/heads/feat/a b", "prunable gitdir file points to non-existent location"},
			),
			want: []model.Worktree{
				{Path: "/r", Head: "aaa", Branch: "main", Main: true},
				{Path: "/r/.claude/worktrees/x", Head: "bbb", Branch: "worktree-x", Locked: true, LockReason: "claude agent agent-1 (pid 42)"},
				{Path: "/codex/1", Head: "ccc", Detached: true},
				{Path: "/gone", Head: "ddd", Branch: "feat/a b", Prunable: true, PruneReason: "gitdir file points to non-existent location"},
			},
		},
		{
			name: "bare main, locked without reason, unknown attribute ignored",
			in: z(
				[]string{"worktree /bare.git", "bare"},
				[]string{"worktree /w", "HEAD eee", "branch refs/heads/b", "locked", "futureattr x"},
			),
			want: []model.Worktree{
				{Path: "/bare.git", Bare: true, Main: true},
				{Path: "/w", Head: "eee", Branch: "b", Locked: true},
			},
		},
		{name: "empty", in: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseWorktreeList(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseWorktreeListRejectsOrphanAttribute(t *testing.T) {
	_, err := ParseWorktreeList([]byte("HEAD aaa\x00"))
	require.Error(t, err)
}

type fakeRunner struct {
	out []byte
	err error
}

func (f fakeRunner) Run(context.Context, string, ...string) ([]byte, error) { return f.out, f.err }

func TestListRepo(t *testing.T) {
	r := fakeRunner{out: z([]string{"worktree /r", "HEAD a", "branch refs/heads/main"}, []string{"worktree /w", "HEAD b", "detached"})}
	repo, err := ListRepo(t.Context(), r, "/r")
	require.NoError(t, err)
	assert.Equal(t, "/r", repo.Root)
	assert.Len(t, repo.Linked(), 1)

	_, err = ListRepo(t.Context(), fakeRunner{err: ErrNotRepo}, "/x")
	assert.True(t, errors.Is(err, ErrNotRepo))
}
