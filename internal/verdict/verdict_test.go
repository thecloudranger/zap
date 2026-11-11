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

// fakeGit answers by the joined args; unknown commands fail loudly.
type fakeGit map[string]error

func (f fakeGit) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	err, ok := f[key]
	if !ok {
		return nil, errors.New("unexpected git " + key)
	}
	return nil, err
}

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
				"merge-base --is-ancestor refs/heads/feat refs/heads/main":          no,
				"merge-base --is-ancestor refs/heads/feat refs/remotes/origin/main": nil,
			},
			false, model.StatusMerged, 2,
		},
		{
			"unmerged runs every check",
			feat,
			fakeGit{
				"merge-base --is-ancestor refs/heads/feat refs/heads/main":          no,
				"merge-base --is-ancestor refs/heads/feat refs/remotes/origin/main": no,
				"diff --quiet refs/heads/main...refs/heads/feat --":                 no,
				"diff --quiet refs/remotes/origin/main...refs/heads/feat --":        no,
			},
			false, model.StatusActive, 3,
		},
		{
			"git error never counts as merged",
			feat,
			fakeGit{
				"merge-base --is-ancestor refs/heads/feat refs/heads/main":          &gitx.CmdError{Code: 128},
				"merge-base --is-ancestor refs/heads/feat refs/remotes/origin/main": &gitx.CmdError{Code: 128},
				"diff --quiet refs/heads/main...refs/heads/feat --":                 &gitx.CmdError{Code: 128},
				"diff --quiet refs/remotes/origin/main...refs/heads/feat --":        &gitx.CmdError{Code: 128},
			},
			false, model.StatusActive, 3,
		},
		{
			"detached head uses the sha; all=true keeps going after a match",
			model.Worktree{Path: "/c", Head: "abcdef1234", Detached: true},
			fakeGit{
				"merge-base --is-ancestor abcdef1234 refs/heads/main": nil,
				"diff --quiet refs/heads/main...abcdef1234 --":        nil,
			},
			true, model.StatusMerged, 3,
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
