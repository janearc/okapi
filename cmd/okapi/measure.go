package main

import (
	"cmp"
	"path/filepath"
	"strings"

	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/measure"
	"github.com/janearc/puzzlemouth/pzm"
)

// measurePage is what okapi measure says of itself.
const measurePage = `# okapi measure NAME=PATH ...

Counts, over each tree, what a plain comment does without: commas,
chained clauses and sentences that open with a or an. Go files that
are not tests and Markdown files are read. The trees are shown side
by side, so one can be the baseline:

    okapi measure mine=. stdlib=$(go env GOROOT)/src

Bare paths are named after their directory.

--at REV counts the files at a git revision, not on disk.
`

// measureVerb is okapi measure: the habits counted over one or more
// trees, side by side.
func (tool okapi) measureVerb(call pzm.Call) error {
	err := need(call, count.One, "okapi measure NAME=PATH ...")
	if err != nil {
		return err
	}
	var names []string
	var counts []measure.Count
	for _, arg := range call.Args() {
		name, path := named(arg)
		tally, err := tool.countTree(path, call.Value("at"))
		if err != nil {
			return err
		}
		names = append(names, name)
		counts = append(counts, tally)
	}
	return measure.Table(call.Out(), names, counts)
}

// named splits NAME=PATH, or names a bare PATH after its directory.
func named(arg string) (string, string) {
	if name, path, ok := strings.Cut(arg, "="); ok {
		return name, path
	}
	abs, _ := filepath.Abs(arg) // empty when there is no working directory
	return filepath.Base(cmp.Or(abs, arg)), arg
}

// countTree tallies one tree, on disk or at a revision.
func (tool okapi) countTree(path, rev string) (measure.Count, error) {
	if rev != "" {
		return measure.At(tool.repo, path, rev)
	}
	return measure.Tree(path)
}
