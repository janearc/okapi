// Package review reads a reviewer's verdicts on the drafts. The
// reviewer gives one verdict for each block. Fix comes with the lines
// the comment should be. Accept takes the draft. Keep leaves the
// original.
package review

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/janearc/okapi/internal/comment"
)

// heading matches the first line of a verdict.
var heading = regexp.MustCompile(
	`^(?P<file>\S+\.go):(?P<line>\d+)\s+(?P<kind>(?i:fix|accept|keep))\s*$`,
)

// The groups of heading, by name.
var (
	fileGroup = heading.SubexpIndex("file")
	lineGroup = heading.SubexpIndex("line")
	kindGroup = heading.SubexpIndex("kind")
)

// isComment matches a line that is a // comment, indented or not.
var isComment = regexp.MustCompile(`^\s*//`)

// Verdict is one block's judgement, and for a fix the new comment.
type Verdict struct {
	kind  string
	lines []string
}

// Kind is fix, accept or keep.
func (verdict Verdict) Kind() string { return verdict.kind }

// Lines is the comment a fix gives, as comment lines.
func (verdict Verdict) Lines() []string { return verdict.lines }

// Verdicts is every verdict in a review, by the block's key.
type Verdicts map[string]Verdict

// Parse reads a review. Each heading reads "FILE:LINE fix",
// "FILE:LINE accept" or "FILE:LINE keep" in either case. The comment
// lines under a fix are the new comment. Anything else is passed over.
func Parse(text string) Verdicts {
	out := Verdicts{}
	key := ""
	for _, line := range strings.Split(text, "\n") {
		if found := heading.FindStringSubmatch(line); found != nil {
			key = found[fileGroup] + ":" + found[lineGroup]
			kind := strings.ToLower(found[kindGroup])
			out[key] = Verdict{kind: kind}
			continue
		}
		verdict, open := out[key]
		if open && verdict.kind == "fix" &&
			isComment.MatchString(line) {
			verdict.lines = append(verdict.lines, line)
			out[key] = verdict
		}
	}
	return out
}

// Of is the verdict on a block, and whether the review gave one.
func (verdicts Verdicts) Of(block comment.Block) (Verdict, bool) {
	verdict, ok := verdicts[block.Key()]
	return verdict, ok
}

// Force sets a verdict by hand. The spec is FILE:LINE=keep, =accept
// or =fix. Fix by hand keeps the lines the review gave.
func (verdicts Verdicts) Force(spec string) error {
	key, kind, ok := strings.Cut(spec, "=")
	kind = strings.ToLower(kind)
	if !ok || (kind != "fix" && kind != "accept" && kind != "keep") {
		return fmt.Errorf(
			"%q is not FILE:LINE=fix, =accept or =keep", spec,
		)
	}
	verdict := verdicts[key]
	verdict.kind = kind
	verdicts[key] = verdict
	return nil
}

// brief is what a reviewer is told, before the list of blocks.
const brief = `Review the comment drafts in %s against the code they
describe.

Each entry below names a Go file and the line its comment starts on.
Open the file there and read three things: the comment as it stands,
the draft in the drafts file, and the code under the comment. Then
give one verdict an entry, in this form and nothing else:

  FILE:LINE fix
  // the comment as it should read, as comment lines
  FILE:LINE accept
  FILE:LINE keep

fix means the draft is wrong or the original could be plainer, and
you give the lines. accept means the draft is right. keep means the
original stays. A draft that claims anything the code does not do is
a fix or a keep, never an accept. Keep the original's indentation.
End with one line: counts: N fix, N accept, N keep.

The entries:
`

// Brief is what a reviewer is told. It says where the drafts are and
// how to read them. It says how to answer and lists the blocks to
// answer for.
func Brief(draftsPath string, blocks []comment.Block) string {
	var text strings.Builder
	fmt.Fprintf(&text, brief, draftsPath)
	for _, block := range blocks {
		fmt.Fprintf(&text, "  %s\n", block.Key())
	}
	return text.String()
}
