// Command okapi makes the comments in a Go tree plain. It measures
// them and shows the blocks worth a rewrite. The local model drafts
// them. Someone reviews the drafts. Then okapi applies the verdicts.
// pzm parses its arguments and writes its help and its errors.
package main

import (
	"os"

	"github.com/janearc/okapi/internal/count"
)

// The codes okapi exits with.
const (
	exitOK     = 0 // defined by the shell: success
	exitFailed = 1 // defined by pzm: the verb ran and failed
	exitUsage  = 2 // defined by pzm: nothing ran
)

// main runs okapi and exits with its code. That is 0 when a verb ran,
// 1 when it failed and 2 when nothing ran.
func main() {
	os.Exit(runPzm(os.Args[count.One:], os.Stdin, os.Stdout, os.Stderr))
}
