package verdict

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/thecloudranger/zap/internal/gitx"
	"github.com/thecloudranger/zap/internal/model"
)

// fakeGit answers by the joined args: a string is stdout, an error is returned, nil is
// a silent success. Unknown commands fail loudly.
type fakeGit map[string]any

func (f fakeGit) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	v, ok := f[key]
	if !ok {
		return nil, errors.New("unexpected git " + key)
	}
	switch v := v.(type) {
	case string:
		return []byte(v), nil
	case error:
		return nil, v
	}
	return nil, nil
}

const (
	featReflog = "reflog show --format=%H refs/heads/feat --"
	headReflog = "reflog show --format=%H HEAD --"
	moved      = "abcdef1234\n1111111111\n"
)

var no = &gitx.CmdError{Code: 1}

func subject(g fakeGit, wt model.Worktree) *Subject {
	return &Subject{
		Git:    g,
		Repo:   model.Repo{Root: "/r"},
		WT:     wt,
		Target: model.Target{Name: "main", Refs: []string{"refs/heads/main", "refs/remotes/origin/main"}},
	}
}

func TestEvaluate(t *testing.T) {
	feat := model.Worktree{Path: "/r/w", Head: "abcdef1234", Branch: "feat"}
	tests := []struct {
		name   string
		wt     model.Worktree
		git    fakeGit
		all    bool
		status model.Status
		checks int
	}{
		{"main worktree", model.Worktree{Main: true}, fakeGit{}, false, model.StatusMain, 0},
		{"on target branch", model.Worktree{Branch: "main", Head: "a"}, fakeGit{}, false, model.StatusActive, 0},
		{"missing short-circuits", model.Worktree{Path: "/x", Head: "a", Branch: "x", Prunable: true}, fakeGit{}, false, model.StatusMissing, 1},
		{
			"merged into origin only",
			feat,
			fakeGit{
				featReflog: moved,
				"merge-base --is-ancestor refs/heads/feat refs/heads/main":          no,
				"merge-base --is-ancestor refs/heads/feat refs/remotes/origin/main": nil,
			},
			false, model.StatusMerged, 3,
		},
		{
			"unmerged runs every check",
			feat,
			fakeGit{
				featReflog: moved,
				"merge-base --is-ancestor refs/heads/feat refs/heads/main":          no,
				"merge-base --is-ancestor refs/heads/feat refs/remotes/origin/main": no,
				"diff --quiet refs/heads/main...refs/heads/feat --":                 no,
				"diff --quiet refs/remotes/origin/main...refs/heads/feat --":        no,
			},
			false, model.StatusActive, 4,
		},
		{
			"git error never counts as merged",
			feat,
			fakeGit{
				featReflog: &gitx.CmdError{Code: 128},
				"merge-base --is-ancestor refs/heads/feat refs/heads/main":          &gitx.CmdError{Code: 128},
				"merge-base --is-ancestor refs/heads/feat refs/remotes/origin/main": &gitx.CmdError{Code: 128},
				"diff --quiet refs/heads/main...refs/heads/feat --":                 &gitx.CmdError{Code: 128},
				"diff --quiet refs/remotes/origin/main...refs/heads/feat --":        &gitx.CmdError{Code: 128},
			},
			false, model.StatusActive, 4,
		},
		{
			"never-moved branch is empty, not merged",
			feat,
			fakeGit{featReflog: "abcdef1234\nabcdef1234\n"},
			false, model.StatusEmpty, 2,
		},
		{
			"detached empty uses the worktree HEAD reflog; all=true keeps going after a match",
			model.Worktree{Path: "/c", Head: "abcdef1234", Detached: true},
			fakeGit{
				headReflog: "abcdef1234\n",
				"merge-base --is-ancestor abcdef1234 refs/heads/main": nil,
				"diff --quiet refs/heads/main...abcdef1234 --":        nil,
			},
			true, model.StatusEmpty, 4,
		},
		{
			"detached that moved and is on target is merged",
			model.Worktree{Path: "/c", Head: "abcdef1234", Detached: true},
			fakeGit{
				headReflog: moved,
				"merge-base --is-ancestor abcdef1234 refs/heads/main": nil,
			},
			false, model.StatusMerged, 3,
		},
		{
			"missing reflog falls through to merge checks",
			feat,
			fakeGit{
				featReflog: "",
				"merge-base --is-ancestor refs/heads/feat refs/heads/main": nil,
			},
			false, model.StatusMerged, 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Evaluate(t.Context(), subject(tt.git, tt.wt), Default(), tt.all)
			assert.Equal(t, tt.status, v.Status, v.Reason)
			assert.Len(t, v.Checks, tt.checks)
			assert.NotEmpty(t, v.Reason)
		})
	}
}

func TestErroredCheckIsReported(t *testing.T) {
	g := fakeGit{
		"merge-base --is-ancestor refs/heads/feat refs/heads/main":          &gitx.CmdError{Code: 128, Stderr: "bad object"},
		"merge-base --is-ancestor refs/heads/feat refs/remotes/origin/main": &gitx.CmdError{Code: 128, Stderr: "bad object"},
	}
	res := Ancestor{}.Eval(t.Context(), subject(g, model.Worktree{Head: "abc", Branch: "feat"}))
	assert.Equal(t, model.Errored, res.Outcome)
	assert.Contains(t, res.Detail, "bad object")
}
