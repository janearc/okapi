// Package apply rewrites the comment blocks of a tree from the drafts
// and the reviewer's verdicts. It goes one file at a time and from the
// bottom up. That keeps the line numbers above a change true.
package apply

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/janearc/okapi/internal/comment"
	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/review"
)

// fileMode is the mode of a new file, defined by unix. The owner reads
// and writes it and everyone else reads it. Files that exist keep the
// mode they have.
const fileMode = 0o644

// Files is where apply reads the trees and writes them back.
type Files interface {
	// Read is the contents of the file at path.
	Read(path string) ([]byte, error)
	// Write replaces the contents of the file at path.
	Write(path string, data []byte) error
}

// disk is the files of this machine.
type disk struct{}

// Disk is the files of this machine.
func Disk() Files { return disk{} }

// Read reads the file at path.
func (disk) Read(path string) ([]byte, error) { return os.ReadFile(path) }

// Write writes the file at path.
func (disk) Write(path string, data []byte) error {
	return os.WriteFile(path, data, fileMode)
}

// counts is how many blocks got each verdict. Blocks the review did
// not mention count as missing and are left as they were.
type counts map[string]int

// kinds is the order counts are told in.
var kinds = []string{"fix", "accept", "keep", "missing"}

// String is the counts as one line.
func (tally counts) String() string {
	parts := make([]string, count.Nothing, len(kinds))
	for _, kind := range kinds {
		parts = append(parts, fmt.Sprintf("%s %d", kind, tally[kind]))
	}
	return strings.Join(parts, "  ")
}

// Tree applies the verdicts to the files under root that the drafts
// name. It goes through files and writes one line on out saying how
// many blocks got each verdict.
//
// Every file is read and placed before any is written. Any block that
// cannot be placed leaves the whole tree as it was.
func Tree(
	files Files, root string, drafts []comment.Block,
	verdicts review.Verdicts, out io.Writer,
) error {
	tally := counts{}
	grouped := byFile(drafts)
	rewritten := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(grouped)) {
		path := filepath.Join(root, name)
		text, err := files.Read(path)
		if err != nil {
			return err
		}
		split := strings.Split(string(text), "\n")
		lines, err := file(split, grouped[name], verdicts, tally)
		if err != nil {
			return err
		}
		rewritten[path] = strings.Join(lines, "\n")
	}
	for path, text := range rewritten {
		if err := files.Write(path, []byte(text)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, tally)
	return err
}

// byFile groups the drafts by the file they are in.
func byFile(drafts []comment.Block) map[string][]comment.Block {
	out := map[string][]comment.Block{}
	for _, block := range drafts {
		out[block.File()] = append(out[block.File()], block)
	}
	return out
}

// lastFirst orders blocks from the last line to the first.
func lastFirst(left, right comment.Block) int {
	return right.Line() - left.Line()
}

// file applies the verdicts to one file's blocks and tallies them. It
// takes the last block first.
func file(
	lines []string, blocks []comment.Block, verdicts review.Verdicts,
	tally counts,
) ([]string, error) {
	blocks = slices.Clone(blocks)
	slices.SortFunc(blocks, lastFirst)
	for _, block := range blocks {
		start, err := placed(lines, block)
		if err != nil {
			return nil, err
		}
		end := start + len(block.Lines())
		verdict, reviewed := verdicts.Of(block)
		kind, replacement := choose(block, verdict, reviewed)
		tally[kind]++
		lines = slices.Replace(lines, start, end, replacement...)
	}
	return lines, nil
}

// notFound is the place of a comment that stands nowhere, defined by the
// slices package: below every place.
const notFound = -1

// twice is the refusal for a comment that moved and now stands in two
// places, and unplaced for one that stands nowhere.
const (
	twice    = "%s: the comment moved and now stands twice; draft again"
	unplaced = "%s: the comment is not what was drafted; draft again"
)

// placed is where a block's comment stands in lines. That is the line
// the draft recorded. When the file has moved under it, that is
// wherever the same lines stand once and only once.
//
// The comment found nowhere is an error. So is the comment found in two
// places. The file changed under the draft.
func placed(lines []string, block comment.Block) (int, error) {
	before := block.Lines()
	if at := block.Line() - count.Origin; matches(lines, at, before) {
		return at, nil
	}
	found := notFound
	for i := range lines {
		if !matches(lines, i, before) {
			continue
		}
		if found != notFound {
			return count.Nothing, fmt.Errorf(twice, block.Key())
		}
		found = i
	}
	if found == notFound {
		return count.Nothing, fmt.Errorf(unplaced, block.Key())
	}
	return found, nil
}

// matches says whether lines begin with want from at on.
func matches(lines []string, at int, want []string) bool {
	return at >= count.First && at+len(want) <= len(lines) &&
		slices.Equal(lines[at:at+len(want)], want)
}

// choose gives the verdict's name and what the block becomes under it.
// Fix takes the reviewer's lines. Accept takes the draft. Keep takes
// the original. Blocks the review passed over are kept and counted as
// missing.
//
// Fix with no lines keeps the original. So does accept with no draft.
// Directives the original held come back at the end when the new
// comment lost them.
func choose(
	block comment.Block, verdict review.Verdict, reviewed bool,
) (string, []string) {
	kind, lines := "missing", block.Lines()
	if reviewed {
		kind = verdict.Kind()
	}
	switch {
	case kind == "fix" && len(verdict.Lines()) > count.Nothing:
		lines = slices.Clone(verdict.Lines())
	case kind == "accept" && block.After() != "":
		lines = strings.Split(block.After(), "\n")
	}
	return kind, withDirectives(block, lines)
}

// withDirectives puts the original's directive lines back after a
// bare // line. It does so only when the new comment has none of its
// own.
func withDirectives(block comment.Block, lines []string) []string {
	kept := block.Directives()
	directed := slices.ContainsFunc(lines, comment.IsDirective)
	if len(kept) == count.Nothing || directed {
		return lines
	}
	separated := append(lines, block.Indent()+"//")
	return append(separated, kept...)
}
