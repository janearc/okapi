package review_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/janearc/okapi/internal/comment"
	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/review"
)

// report is a review with every kind of verdict and some noise.
const report = `Here is the review.

a/b.go:12 fix
// the comment as it should read
// on two lines
a/b.go:40 accept
// a stray line under an accept is not kept
c.go:7 KEEP
d.go:1 Fix

counts: 2 fix, 1 accept, 1 keep
e.go:9 fixed
`

// reported is the keys of the verdicts in report.
var reported = []string{"a/b.go:12", "a/b.go:40", "c.go:7", "d.go:1"}

// blockAt is a one-line comment block in file at line, which is a
// number written out.
func blockAt(t *testing.T, file, line string) comment.Block {
	t.Helper()
	at, err := strconv.Atoi(line)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("var x\n", at-count.One) + "// x\n"
	blocks := comment.Blocks(file, text)
	return blocks[len(blocks)-count.One]
}

// headings in either case open a verdict; only a fix keeps lines.
func TestParse(t *testing.T) {
	verdicts := review.Parse(report)
	if len(verdicts) != len(reported) {
		t.Fatalf("got %d verdicts: %v", len(verdicts), verdicts)
	}
	fix := verdicts["a/b.go:12"]
	if fix.Kind() != "fix" || !slices.Equal(fix.Lines(), []string{
		"// the comment as it should read", "// on two lines",
	}) {
		t.Errorf("fix: %q %q", fix.Kind(), fix.Lines())
	}
	accept := verdicts["a/b.go:40"]
	if accept.Kind() != "accept" || accept.Lines() != nil {
		t.Errorf("accept: %q %q", accept.Kind(), accept.Lines())
	}
	if verdicts["c.go:7"].Kind() != "keep" {
		t.Errorf("keep: %q", verdicts["c.go:7"].Kind())
	}
}

// headings with no lines under them are bare fixes. Headings that only
// start with a verdict word are no headings.
func TestParseBareAndNearHeadings(t *testing.T) {
	verdicts := review.Parse(report)
	bare := verdicts["d.go:1"]
	if bare.Kind() != "fix" || bare.Lines() != nil {
		t.Errorf("bare fix: %q %q", bare.Kind(), bare.Lines())
	}
	if _, ok := verdicts["e.go:9"]; ok {
		t.Error("fixed is not a verdict")
	}
}

// blocks find their verdict by file and line.
func TestOf(t *testing.T) {
	verdicts := review.Parse(report)
	block := blockAt(t, "a/b.go", "40")
	verdict, ok := verdicts.Of(block)
	if !ok || verdict.Kind() != "accept" {
		t.Errorf("got %v %v", verdict, ok)
	}
	if _, ok := verdicts.Of(blockAt(t, "a/b.go", "41")); ok {
		t.Error("line 41 has no verdict")
	}
}

// verdicts forced by hand replace the kind and keep a fix's lines.
func TestForce(t *testing.T) {
	verdicts := review.Parse(report)
	was := len(verdicts["a/b.go:12"].Lines())
	if err := verdicts.Force("a/b.go:12=KEEP"); err != nil {
		t.Fatal(err)
	}
	forced := verdicts["a/b.go:12"]
	if forced.Kind() != "keep" || len(forced.Lines()) != was {
		t.Errorf("forced: %q %q", forced.Kind(), forced.Lines())
	}
	if err := verdicts.Force("new.go:3=accept"); err != nil {
		t.Fatal(err)
	}
	if verdicts["new.go:3"].Kind() != "accept" {
		t.Errorf("new: %q", verdicts["new.go:3"].Kind())
	}
}

// forcing something that is not a verdict is refused by name.
func TestForceRefusesWhatIsNotAVerdict(t *testing.T) {
	verdicts := review.Parse(report)
	for _, bad := range []string{"a.go:1", "a.go:1=", "a.go:1=maybe"} {
		if err := verdicts.Force(bad); err == nil ||
			!strings.Contains(err.Error(), bad) {
			t.Errorf("Force(%q): %v", bad, err)
		}
	}
}

// the brief says where the drafts are and gives the three words. It
// lists every block in order.
func TestBrief(t *testing.T) {
	blocks := []comment.Block{blockAt(t, "a.go", "1"),
		blockAt(t, "b/c.go", "22")}
	text := review.Brief("out/drafts.json", blocks)
	for _, want := range []string{
		"drafts in out/drafts.json", "FILE:LINE fix",
		"FILE:LINE accept", "FILE:LINE keep", "counts:",
		"\n  a.go:1\n  b/c.go:22\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the brief lacks %q:\n%s", want, text)
		}
	}
}
