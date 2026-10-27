package main

import (
	"fmt"
	"os"

	"github.com/janearc/okapi/internal/apply"
	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/draft"
	"github.com/janearc/okapi/internal/review"
	"github.com/janearc/puzzlemouth/pzm"
)

// briefPage is what okapi brief says of itself.
const briefPage = `# okapi brief DRAFTS.json

Prints what a reviewer is told: where the drafts are, how to read each
against its code, how to answer, and the blocks to answer for. The
answer is the REVIEW that okapi apply takes.
`

// briefVerb is okapi brief: the reviewer's instructions for a drafts
// file.
func briefVerb(call pzm.Call) error {
	err := need(call, count.One, "okapi brief DRAFTS.json")
	if err != nil {
		return err
	}
	path := call.Args()[count.First]
	drafts, err := draft.Read(path)
	if err != nil {
		return err
	}
	brief := review.Brief(path, drafts)
	fmt.Fprint(call.Out(), brief)
	return nil
}

// applyPage is what okapi apply says of itself.
const applyPage = `# okapi apply [--repo DIR] DRAFTS REVIEW [FILE:LINE=KIND ...]

Rewrites the files the drafts name from the reviewer's verdicts.
Fix puts in the reviewer's lines. Accept puts in the draft. Keep
leaves the original. Blocks the review passed over are kept and
counted as missing.

FILE:LINE=keep, =accept or =fix after the review overrides one
verdict. Lines that instruct a tool, such as go:embed, are kept.

--repo DIR is the tree the drafts' paths are under; the default is the
current directory.

--dry-run says how many drafted blocks the review would be applied to,
and writes nothing.
`

// applyNeeds is how many words apply takes before its overrides: the
// drafts and the review.
const applyNeeds = count.Pair

// applyVerb is okapi apply: the verdicts applied to the tree, and the
// counts.
func (tool okapi) applyVerb(call pzm.Call) error {
	err := need(call, applyNeeds, "okapi apply DRAFTS.json REVIEW")
	if err != nil {
		return err
	}
	root := "."
	if call.Given("repo") {
		root = call.Value("repo")
	}
	drafts, err := draft.Read(call.Args()[count.First])
	if err != nil {
		return err
	}
	text, err := os.ReadFile(call.Args()[count.One])
	if err != nil {
		return err
	}
	verdicts := review.Parse(string(text))
	for _, spec := range call.Args()[applyNeeds:] {
		if err := verdicts.Force(spec); err != nil {
			return err
		}
	}
	if call.DryRun() {
		_, err = fmt.Fprintf(call.Out(),
			"would apply the review to %d blocks under %s\n",
			len(drafts), root)
		return err
	}
	return apply.Tree(tool.files, root, drafts, verdicts, call.Out())
}
