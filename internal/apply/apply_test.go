package apply_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/janearc/okapi/internal/apply"
	"github.com/janearc/okapi/internal/comment"
	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/review"
	"go.uber.org/mock/gomock"
	"pgregory.net/rapid"
)

// The modes tests write files with.
const (
	fileMode = 0o644 // defined by unix: the owner writes, everyone reads
	readOnly = 0o444 // defined by unix: nobody writes
)

// source is a file with four blocks: one for each verdict and one the
// review passes over.
const source = `package p

// one, with, commas.
// and a second line
func one() {}

// two is long.
// it goes on.
func two() {}

	// three stays, as it is.
	var three = 3

// four, with a directive.
//go:embed x
var four embed.FS
`

// rewritten is source after the verdicts in report.
const rewritten = `package p

// one is fixed.
func one() {}

// two is drafted.
func two() {}

	// three stays, as it is.
	var three = 3

// four, with a directive.
//go:embed x
var four embed.FS
`

// report is the review of source's blocks, less the fourth.
const report = `p.go:3 fix
// one is fixed.
p.go:7 accept
p.go:11 keep
`

// root is the tree the mock files hold.
const root = "tree"

// written is a gomock matcher that accepts any bytes and keeps them as
// text, so a test can read what was written.
type written struct{ text string }

// Matches keeps the bytes.
func (put *written) Matches(value any) bool {
	data, ok := value.([]byte)
	put.text = string(data)
	return ok
}

// String says what the matcher accepts.
func (put *written) String() string { return "any bytes" }

// drafts is source's blocks, with a draft on the second and third.
func drafts(t *testing.T) []comment.Block {
	t.Helper()
	blocks := comment.Blocks("p.go", source)
	if len(blocks) <= count.Pair+count.One {
		t.Fatalf("%d blocks in source", len(blocks))
	}
	blocks[count.One].Drafted("// two is drafted.")
	blocks[count.Pair].Drafted("\t// three is drafted, but kept.")
	return blocks
}

// held is a mock tree that holds p.go with text.
func held(t *testing.T, text string) *MockFiles {
	t.Helper()
	files := NewMockFiles(gomock.NewController(t))
	files.EXPECT().Read(filepath.Join(root, "p.go")).
		Return([]byte(text), nil)
	return files
}

// applied runs Tree over a mock tree holding text as p.go, which must
// be written once. It returns the text written and the line Tree said.
func applied(
	t *testing.T, text string, blocks []comment.Block,
	verdicts review.Verdicts,
) (string, string, error) {
	t.Helper()
	files := held(t, text)
	put := &written{}
	files.EXPECT().Write(filepath.Join(root, "p.go"), put).Return(nil)
	var out bytes.Buffer
	err := apply.Tree(files, root, blocks, verdicts, &out)
	return put.text, strings.TrimSpace(out.String()), err
}

// refused runs Tree over a mock tree holding text as p.go, which must
// not be written. It returns what Tree said and its error.
func refused(
	t *testing.T, text string, blocks []comment.Block,
	verdicts review.Verdicts,
) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := apply.Tree(held(t, text), root, blocks, verdicts, &out)
	return out.String(), err
}

// the file is rewritten under its verdicts and the line says what
// happened.
func TestTree(t *testing.T) {
	after, said, err := applied(t, source, drafts(t), review.Parse(report))
	if err != nil {
		t.Fatal(err)
	}
	if said != "fix 1  accept 1  keep 1  missing 1" {
		t.Errorf("said %q", said)
	}
	if after != rewritten {
		t.Errorf("got:\n%s", after)
	}
}

// the tree refuses a missing file and writes nothing.
func TestTreeMissingFile(t *testing.T) {
	files := NewMockFiles(gomock.NewController(t))
	files.EXPECT().Read(filepath.Join(root, "p.go")).
		Return(nil, fs.ErrNotExist)
	var out bytes.Buffer
	err := apply.Tree(files, root, drafts(t), nil, &out)
	if !errors.Is(err, fs.ErrNotExist) || out.Len() != count.Nothing {
		t.Errorf("a missing file: %v, said %q", err, out.String())
	}
}

// the tree reports a file it cannot write and says nothing.
func TestTreeUnwritableFile(t *testing.T) {
	files := held(t, source)
	refusal := errors.New("read-only")
	files.EXPECT().Write(filepath.Join(root, "p.go"), gomock.Any()).
		Return(refusal)
	var out bytes.Buffer
	err := apply.Tree(files, root, drafts(t), nil, &out)
	if !errors.Is(err, refusal) || out.Len() != count.Nothing {
		t.Errorf("an unwritable file: %v, said %q", err, out.String())
	}
}

// the first line of a file holds a block like any other.
func TestTreeFirstLine(t *testing.T) {
	first := "// first, with, commas.\npackage p\n"
	blocks := comment.Blocks("p.go", first)
	after, said, err := applied(t, first, blocks, review.Parse("p.go:1 keep"))
	if err != nil || after != first || said != "fix 0  accept 0  keep 1  "+
		"missing 0" {
		t.Errorf("%q, %v\n%s", said, err, after)
	}
}

// files that moved under the drafts are still applied, each comment
// found where it now stands.
func TestTreeMoved(t *testing.T) {
	moved := "\n\n" + source
	after, said, err := applied(t, moved, drafts(t), review.Parse(report))
	want := "\n\n" + rewritten
	if err != nil || after != want ||
		said != "fix 1  accept 1  keep 1  missing 1" {
		t.Errorf("moved: %q, %v\n%s", said, err, after)
	}
}

// files with no package line above the first block are applied.
func TestTreeHeadless(t *testing.T) {
	headless := strings.TrimPrefix(source, "package p\n\n")
	after, said, err := applied(t, headless, drafts(t), review.Parse(report))
	want := strings.TrimPrefix(rewritten, "package p\n\n")
	if err != nil || after != want ||
		said != "fix 1  accept 1  keep 1  missing 1" {
		t.Errorf("headless: %q, %v\n%s", said, err, after)
	}
}

// blocks the review never mentions are kept and counted as missing.
func TestTreeNothingReviewed(t *testing.T) {
	after, said, err := applied(t, source, drafts(t), review.Verdicts{})
	if err != nil || after != source ||
		said != "fix 0  accept 0  keep 0  missing 4" {
		t.Errorf("nothing reviewed: %q, %v", said, err)
	}
}

// changed comments are refused by name, and nothing is written.
func TestTreeRefusesAChangedComment(t *testing.T) {
	changed := strings.ReplaceAll(source, "// two is long.", "// two.")
	said, err := refused(t, changed, drafts(t), review.Verdicts{})
	if err == nil || !strings.Contains(err.Error(), "p.go:7:") ||
		said != "" {
		t.Errorf("changed: %v, said %q", err, said)
	}
}

// comments that now stand twice are refused, and nothing is written.
func TestTreeRefusesACommentThatStandsTwice(t *testing.T) {
	dup := "// one, with, commas.\n// and a second line\n"
	_, err := refused(t, dup+source, drafts(t), review.Verdicts{})
	if err == nil || !strings.Contains(err.Error(), "stands twice") {
		t.Errorf("twice: %v", err)
	}
}

// blocks past the end of the file are refused.
func TestTreeRefusesABlockPastTheEnd(t *testing.T) {
	short := "package p\n\n// one, with, commas.\n"
	if _, err := refused(t, short, drafts(t), review.Verdicts{}); err == nil {
		t.Error("a block past the end is an error")
	}
}

// nothing is written until every file is placed: a second file that
// refuses leaves the first as it was.
func TestTreeWritesNothingOnARefusal(t *testing.T) {
	files := NewMockFiles(gomock.NewController(t))
	files.EXPECT().Read(filepath.Join(root, "p.go")).
		Return([]byte(source), nil)
	files.EXPECT().Read(filepath.Join(root, "q.go")).
		Return([]byte("package q\n"), nil)
	blocks := append(drafts(t), comment.Blocks("q.go", "// gone.\n")...)
	var out bytes.Buffer
	err := apply.Tree(files, root, blocks, review.Parse(report), &out)
	if err == nil || !strings.Contains(err.Error(), "q.go:1:") {
		t.Fatalf("a changed file was not refused: %v", err)
	}
	if out.Len() != count.Nothing {
		t.Errorf("said %q before a file refused", out.String())
	}
}

// each verdict chooses lines, with the original as the fallback and
// its directives kept.
func TestTreeChooses(t *testing.T) {
	blocks := drafts(t)
	blocks[count.Pair+count.One].Drafted("// four is drafted.")
	verdicts := review.Parse("p.go:3 fix\np.go:7 fix\n//go:embed y\n" +
		"p.go:11 accept\np.go:14 accept\n")
	after, said, err := applied(t, source, blocks, verdicts)
	if err != nil {
		t.Fatal(err)
	}
	want := `package p

// one, with, commas.
// and a second line
func one() {}

//go:embed y
func two() {}

	// three is drafted, but kept.
	var three = 3

// four is drafted.
//
//go:embed x
var four embed.FS
`
	if after != want || said != "fix 2  accept 2  keep 0  missing 0" {
		t.Errorf("%s\n%s", said, after)
	}
}

// the disk reads back what it wrote. new files are no more open than
// the owner writing and everyone reading. missing files are said to be
// missing.
func TestDiskWritesAndReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.go")
	disk := apply.Disk()
	if err := disk.Write(path, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	data, err := disk.Read(path)
	if err != nil || string(data) != "hello" {
		t.Errorf("read back %q, %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&^fileMode != count.Nothing {
		t.Errorf("mode %v, %v", info.Mode(), err)
	}
	missing := filepath.Join(filepath.Dir(path), "none")
	if _, err := disk.Read(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file: %v", err)
	}
}

// the disk cannot write a file that nobody may write.
func TestDiskCannotWriteAReadOnlyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.go")
	if err := os.WriteFile(path, nil, readOnly); err != nil {
		t.Fatal(err)
	}
	if err := apply.Disk().Write(path, []byte("x")); err == nil {
		t.Error("a file nobody may write was written")
	}
}

// arrange draws a file of code runs and comment blocks with a verdict
// for each block. It returns the file's text and the text it should
// become. It returns the review and the blocks with their drafts.
func arrange(check *rapid.T) (string, string, string, []comment.Block) {
	codeLine := rapid.StringMatching(`[a-z]{1,8}`)
	commentLine := rapid.StringMatching(`// [a-z]{1,12}`)
	var lines, want, afters []string
	var report strings.Builder
	specs := rapid.SliceOf(commentLine).Filter(someLines)
	for place := range rapid.SliceOf(specs).Filter(someSpecs).Draw(check, "blocks") {
		code := rapid.SliceOf(codeLine).Draw(check, "code")
		if place > count.First && len(code) == count.Nothing {
			code = []string{"filler"}
		}
		lines, want = append(lines, code...), append(want, code...)
		before := specs.Draw(check, "before")
		key := "a.go:" + strconv.Itoa(len(lines)+count.Origin)
		lines = append(lines, before...)
		other := specs.Draw(check, "other")
		kind := rapid.SampledFrom(
			[]string{"fix", "accept", "keep", "missing"},
		).Draw(check, "kind")
		afters = append(afters, "")
		switch kind {
		case "fix":
			report.WriteString(key + " fix\n" +
				strings.Join(other, "\n") + "\n")
			want = append(want, other...)
		case "accept":
			afters[len(afters)-count.One] = strings.Join(other, "\n")
			report.WriteString(key + " accept\n")
			want = append(want, other...)
		case "keep":
			report.WriteString(key + " keep\n")
			want = append(want, before...)
		default:
			want = append(want, before...)
		}
	}
	tail := rapid.SliceOf(codeLine).Draw(check, "tail")
	lines, want = append(lines, tail...), append(want, tail...)
	text := strings.Join(lines, "\n")
	blocks := comment.Blocks("a.go", text)
	if len(blocks) != len(afters) {
		check.Fatalf("%d blocks read from:\n%s", len(blocks), text)
	}
	for place := range blocks {
		blocks[place].Drafted(afters[place])
	}
	return text, strings.Join(want, "\n"), report.String(), blocks
}

// someLines is whether a run of comment lines has any.
func someLines(lines []string) bool { return len(lines) > count.Nothing }

// someSpecs is whether a run of comment blocks has any.
func someSpecs(specs [][]string) bool { return len(specs) > count.Nothing }

// whatever the blocks and verdicts, the code between them is kept in
// order and every block is counted once.
// keepsTheCode is the property of TestTreeKeepsTheCode.
func keepsTheCode(check *rapid.T) {
	text, want, report, blocks := arrange(check)
	files := NewMockFiles(gomock.NewController(check))
	files.EXPECT().Read(filepath.Join(root, "a.go")).
		Return([]byte(text), nil)
	put := &written{}
	files.EXPECT().Write(filepath.Join(root, "a.go"), put).Return(nil)
	var out bytes.Buffer
	err := apply.Tree(files, root, blocks, review.Parse(report), &out)
	if err != nil {
		check.Fatal(err)
	}
	if put.text != want {
		check.Fatalf("got\n%s\nwant\n%s", put.text, want)
	}
	var fix, accept, keep, missing int
	_, err = fmt.Sscanf(out.String(),
		"fix %d  accept %d  keep %d  missing %d",
		&fix, &accept, &keep, &missing)
	if err != nil || fix+accept+keep+missing != len(blocks) {
		check.Fatalf("said %q for %d blocks",
			out.String(), len(blocks))
	}
}

func TestTreeKeepsTheCode(t *testing.T) {
	rapid.Check(t, keepsTheCode)
}
