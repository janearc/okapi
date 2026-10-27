package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/janearc/okapi/internal/comment"
	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/puzzlemouth/pzm"
)

// blocksPage is what okapi blocks says of itself.
const blocksPage = `# okapi blocks [--all] [--history WORDS] FILE ...

Prints the comment blocks in each Go file that have a habit a plain
comment does without. The habit is named: commas, chain, opener, or
history for a comment that tells the code's past.

Blocks whose only mark is that they run past one line are shown with
--all. Draft sends those too.

--history WORDS adds more words that mark a comment as history, comma
separated.
`

// history matches a comment that tells the code's past instead of its
// present.
var history = regexp.MustCompile(
	`20\d\d-\d\d-\d\d|first draft|proposal|was tried|used to`,
)

// blocksVerb is okapi blocks: the blocks worth a rewrite, printed with
// their reasons.
func blocksVerb(call pzm.Call) error {
	err := need(call, count.One, "okapi blocks FILE ...")
	if err != nil {
		return err
	}
	marks := historyWith(call.Value("history"))
	for _, path := range call.Args() {
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, block := range comment.Blocks(path, string(text)) {
			reasons := block.Reasons()
			if marks.MatchString(block.Before()) {
				reasons = append(reasons, "history")
			}
			if shown(reasons, call.On("all")) {
				show(call.Out(), block, reasons)
			}
		}
	}
	return nil
}

// historyWith is the history pattern with more words added, each
// matched as it is.
func historyWith(words string) *regexp.Regexp {
	pattern := history.String()
	for _, word := range strings.Split(words, ",") {
		if word = strings.TrimSpace(word); word != "" {
			pattern += "|" + regexp.QuoteMeta(word)
		}
	}
	return regexp.MustCompile(pattern)
}

// shown says whether a block with these reasons is printed: any reason
// but long alone, or anything at all with --all.
func shown(reasons []string, all bool) bool {
	if len(reasons) == count.Nothing {
		return false
	}
	return all || len(reasons) > count.One || reasons[count.First] != "long"
}

// show prints one block with its place, its reasons and its lines.
func show(out io.Writer, block comment.Block, reasons []string) {
	last := block.Line() + len(block.Lines()) - count.One
	fmt.Fprintf(
		out, "== %s:%d-%d  %s\n%s\n",
		block.File(), block.Line(), last, strings.Join(reasons, ", "),
		block.Before(),
	)
}
