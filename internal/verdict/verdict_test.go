package verdict

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
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
	featUpstream = "for-each-ref --format=%(upstream:short)%00%(upstream:track) refs/heads/feat"
	cherryMain   = "cherry refs/heads/main refs/heads/feat"
	cherryOrigin = "cherry refs/remotes/origin/main refs/heads/feat"
	mtMain       = "merge-tree --write-tree --no-messages refs/heads/main refs/heads/feat"
	mtOrigin     = "merge-tree --write-tree --no-messages refs/remotes/origin/main refs/heads/feat"
	featReflog   = "reflog show --format=%H%x00%gs refs/heads/feat --"
	headReflog   = "reflog show --format=%H%x00%gs HEAD --"
	moved        = "abcdef1234\n1111111111\n"
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
				cherryMain:   "+ abcdef1234\n",
				cherryOrigin: "+ abcdef1234\n",
				mtMain:       no, // conflicts
				mtOrigin:     no,
				featUpstream: "origin/feat\x00\n",
			},
			false, model.StatusActive, 8,
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
				cherryMain:   &gitx.CmdError{Code: 128},
				cherryOrigin: &gitx.CmdError{Code: 128},
				mtMain:       &gitx.CmdError{Code: 128},
				mtOrigin:     &gitx.CmdError{Code: 128},
				featUpstream: &gitx.CmdError{Code: 128},
			},
			false, model.StatusActive, 8,
		},
		{
			"never-moved branch is empty, not merged",
			feat,
			fakeGit{
				featReflog: "abcdef1234\x00reset: moving to HEAD\nabcdef1234\x00branch: Created from HEAD\n",
				"merge-base --is-ancestor refs/heads/feat refs/heads/main": nil,
			},
			false, model.StatusEmpty, 2,
		},
		{
			"never moved but created from unmerged work is not empty",
			feat,
			fakeGit{
				featReflog: "abcdef1234\x00branch: Created from origin/feature\n",
				"merge-base --is-ancestor refs/heads/feat refs/heads/main":          no,
				"merge-base --is-ancestor refs/heads/feat refs/remotes/origin/main": no,
				"diff --quiet refs/heads/main...refs/heads/feat --":                 no,
				"diff --quiet refs/remotes/origin/main...refs/heads/feat --":        no,
				cherryMain:   "+ abcdef1234\n",
				cherryOrigin: "+ abcdef1234\n",
				mtMain:       no,
				mtOrigin:     no,
				featUpstream: "origin/feature\x00\n",
			},
			false, model.StatusActive, 8,
		},
		{
			"a branch zap undo restored is judged by the merge checks, not as empty",
			feat,
			fakeGit{
				featReflog: "abcdef1234\x00branch: Created from refs/zap/backup/20261113T101500/feat\n",
				"merge-base --is-ancestor refs/heads/feat refs/heads/main": nil,
			},
			false, model.StatusMerged, 3,
		},
		{
			"detached empty uses the worktree HEAD reflog; all=true keeps going after a match",
			model.Worktree{Path: "/c", Head: "abcdef1234", Detached: true},
			fakeGit{
				headReflog: "abcdef1234\n",
				"merge-base --is-ancestor abcdef1234 refs/heads/main":              nil,
				"diff --quiet refs/heads/main...abcdef1234 --":                     nil,
				"cherry refs/heads/main abcdef1234":                                "",
				"cherry refs/remotes/origin/main abcdef1234":                       "",
				"merge-tree --write-tree --no-messages refs/heads/main abcdef1234": "t1\n",
				"rev-parse --verify refs/heads/main^{tree}":                        "t1\n",
			},
			true, model.StatusEmpty, 8,
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
			"rebase merge: every patch already in target",
			feat,
			fakeGit{
				featReflog: moved,
				"merge-base --is-ancestor refs/heads/feat refs/heads/main":          no,
				"merge-base --is-ancestor refs/heads/feat refs/remotes/origin/main": no,
				"diff --quiet refs/heads/main...refs/heads/feat --":                 no,
				"diff --quiet refs/remotes/origin/main...refs/heads/feat --":        no,
				cherryMain: "- abcdef1234\n- 2222222222\n",
			},
			false, model.StatusMerged, 5,
		},
		{
			"squash merge: merging adds nothing; old git skips the check",
			feat,
			fakeGit{
				featReflog: moved,
				"merge-base --is-ancestor refs/heads/feat refs/heads/main":          no,
				"merge-base --is-ancestor refs/heads/feat refs/remotes/origin/main": no,
				"diff --quiet refs/heads/main...refs/heads/feat --":                 no,
				"diff --quiet refs/remotes/origin/main...refs/heads/feat --":        no,
				cherryMain:   "+ abcdef1234\n",
				cherryOrigin: "+ abcdef1234\n",
				mtMain:       &gitx.CmdError{Code: 129},
				featUpstream: "origin/feat\x00[gone]\n",
			},
			false, model.StatusGone, 8,
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

func TestSquashMatchesOnEqualTree(t *testing.T) {
	g := fakeGit{
		mtMain: "t9\n",
		"rev-parse --verify refs/heads/main^{tree}": "t9\n",
	}
	res := Squash{}.Eval(t.Context(), subject(g, model.Worktree{Head: "abcdef1234", Branch: "feat"}))
	assert.Equal(t, model.Match, res.Outcome, res.Detail)
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

func TestPullRequest(t *testing.T) {
	const tip = "abcdef1234567890"
	wt := model.Worktree{Head: tip, Branch: "feat"}
	call := "pr list --state merged --head feat --json number,headRefOid,baseRefName --limit 20"
	eval := func(gh fakeGit) model.CheckResult {
		s := subject(fakeGit{}, wt)
		if gh != nil {
			s.GH = gh
		}
		return PullRequest{}.Eval(t.Context(), s)
	}
	assert.Equal(t, model.Skipped, eval(nil).Outcome, "off unless asked")
	assert.Equal(t, model.Match, eval(fakeGit{call: `[{"number":7,"headRefOid":"` + tip + `","baseRefName":"main"}]`}).Outcome)
	r := eval(fakeGit{call: `[{"number":7,"headRefOid":"0000000","baseRefName":"main"}]`})
	assert.Equal(t, model.NoMatch, r.Outcome, "commits were added after the merge")
	assert.Contains(t, r.Detail, "#7")
	assert.Equal(t, model.NoMatch, eval(fakeGit{call: `[{"number":8,"headRefOid":"` + tip + `","baseRefName":"release"}]`}).Outcome, "merged elsewhere")
	assert.Equal(t, model.NoMatch, eval(fakeGit{call: `[]`}).Outcome)
	assert.Equal(t, model.Skipped, eval(fakeGit{call: fmt.Errorf("gitx: %w", exec.ErrNotFound)}).Outcome)
	assert.Equal(t, model.Errored, eval(fakeGit{call: &gitx.CmdError{Code: 1, Stderr: "auth required"}}).Outcome)
}
