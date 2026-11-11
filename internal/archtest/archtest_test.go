// Package archtest enforces zap's dependency rules with a plain Go test, so the rules
// cannot drift with linter versions.
package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/thecloudranger/zap/"

// allowed lists, per package, the internal packages it may import. Packages not listed
// may import any internal package except those guarded by importers below.
var allowed = map[string][]string{
	"internal/model":    {},
	"internal/version":  {},
	"internal/gitx":     {"internal/model"},
	"internal/verdict":  {"internal/model", "internal/gitx"},
	"internal/inspect":  {"internal/model", "internal/gitx", "internal/verdict", "internal/agent", "internal/sizer"},
	"internal/sizer":    {"internal/model"},
	"internal/tui":      {"internal/model", "internal/plan", "internal/sizer"},
	"internal/agent":    {},
	"internal/discover": {},
	"internal/policy":   {"internal/model"},
	"internal/safety":   {"internal/model", "internal/policy", "internal/procs"},
	"internal/procs":    {},
	"internal/oplog":    {},
	"internal/restore":  {"internal/model", "internal/gitx", "internal/oplog"},
	"internal/plan": {"internal/model", "internal/gitx", "internal/inspect", "internal/verdict",
		"internal/policy", "internal/safety", "internal/oplog", "internal/sizer", "internal/procs"},
}

// importers restricts who may import a package.
var importers = map[string][]string{
	"internal/cli":     {"cmd/zap"},
	"internal/testkit": {}, // test code only
}

func TestDependencyRules(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	imports := collect(t, root)
	for pkg, deps := range imports {
		for dep := range deps {
			if allow, ok := allowed[pkg]; ok && !slices.Contains(allow, dep) {
				t.Errorf("%s must not import %s", pkg, dep)
			}
			if who, ok := importers[dep]; ok && !slices.Contains(who, pkg) {
				t.Errorf("%s must not import %s (only %v may)", pkg, dep, who)
			}
		}
	}
	if len(imports) == 0 {
		t.Fatal("no packages found; is the root path right?")
	}
}

// collect maps each package dir (relative to root) to the internal packages its
// non-test files import.
func collect(t *testing.T, root string) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "testdata") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, p, src, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		rel = filepath.ToSlash(rel)
		if out[rel] == nil {
			out[rel] = map[string]bool{}
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if dep, ok := strings.CutPrefix(path, module); ok {
				out[rel][dep] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
