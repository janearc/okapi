package main

import (
	"bytes"
	"fmt"
	"strings"
)

// said runs okapi and returns what it said, or panics with what it
// said on stderr when it failed.
func said(args ...string) string {
	var out, errs bytes.Buffer
	code := runPzm(args, strings.NewReader(""), &out, &errs)
	if code != exitOK {
		panic(errs.String())
	}
	return out.String()
}

func Example_measure() {
	fmt.Print(said("measure", "mine=testdata/tree"))
	// Output:
	//                      mine
	// files                   2
	// sentences               6
	// commas a sentence    0.67
	// two or more commas  16.7%
	// open with a or an   16.7%
	// chain clauses       16.7%
}

func Example_blocks() {
	fmt.Print(said("blocks", "testdata/tree/plain.go"))
	// Output:
	// == testdata/tree/plain.go:6-7  long, commas, chain, opener
	// // A habit opens this sentence, and it chains on, and on again, until
	// // nobody is left.
}

func Example_draft() {
	fmt.Print(said("draft", "--dry-run", "--model", "example",
		"drafts.json", "testdata/tree/plain.go"))
	// Output:
	// would draft 2 blocks with example to drafts.json
}

func Example_brief() {
	brief := said("brief", "testdata/drafts.json")
	opening, _, _ := strings.Cut(brief, "\n\n")
	fmt.Println(opening)
	// Output:
	// Review the comment drafts in testdata/drafts.json against the code they
	// describe.
}

func Example_apply() {
	fmt.Print(said("apply", "--dry-run", "testdata/drafts.json",
		"testdata/review.txt"))
	// Output:
	// would apply the review to 1 blocks under .
}

func Example_version() {
	help := said("version", "--help")
	fmt.Println(firstLine(help))
	// Output:
	// okapi version [ARGS]: says which okapi this is
}
