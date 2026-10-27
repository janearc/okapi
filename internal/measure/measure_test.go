package measure_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/measure"
	"go.uber.org/mock/gomock"
)

// The modes tests write with, defined by unix.
const (
	fileMode = 0o644 // defined by unix: the owner writes, everyone reads
	dirMode  = 0o755 // defined by unix: a folder anyone can enter
	noMode   = 0o000 // defined by unix: nobody may read or write
)

// three is a Go file with three comment sentences. One is plain. One
// has every habit. One is an opener with two commas.
const three = `// a plain one here. one, two, and three, because four, so five.
// An opener, here, with two.
package three
`

// one is a Go file with one comment sentence holding one comma.
const one = `// one sentence, here.
package one
`

// table is the counts of trees as Table prints them, split into rows
// of fields.
func table(t *testing.T, names []string, counts ...measure.Count) (
	string, [][]string,
) {
	t.Helper()
	var out bytes.Buffer
	if err := measure.Table(&out, names, counts); err != nil {
		t.Fatal(err)
	}
	var rows [][]string
	text := strings.TrimRight(out.String(), "\n")
	for _, line := range strings.Split(text, "\n") {
		rows = append(rows, strings.Fields(line))
	}
	return out.String(), rows
}

// the table has a row for each label. The numbers are the tally.
func TestTreeAndTable(t *testing.T) {
	root := t.TempDir()
	write(t, root, "three.go", three)
	tally, err := measure.Tree(root)
	if err != nil {
		t.Fatal(err)
	}
	text, rows := table(t, []string{"mine"}, tally)
	want := [][]string{
		{"mine"},
		{"files", "1"},
		{"sentences", "3"},
		{"commas", "a", "sentence", "2.00"},
		{"two", "or", "more", "commas", "66.7%"},
		{"open", "with", "a", "or", "an", "66.7%"},
		{"chain", "clauses", "33.3%"},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows:\n%s", len(rows), text)
	}
	for place, row := range rows {
		if !slices.Equal(row, want[place]) {
			t.Errorf("row %d is %q, want %q", place, row, want[place])
		}
	}
}

// the columns line up: every row is as wide as the header.
func TestTableLinesUp(t *testing.T) {
	root := t.TempDir()
	write(t, root, "three.go", three)
	tally, err := measure.Tree(root)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := table(t, []string{"mine"}, tally)
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for place, line := range lines {
		if len(line) != len(lines[count.First]) {
			t.Errorf("row %d is %d wide, the header %d", place, len(line),
				len(lines[count.First]))
		}
	}
}

// two columns are each as wide as their widest cell or name. One
// sentence is divided by one.
func TestTableTwoColumns(t *testing.T) {
	root := t.TempDir()
	write(t, root, "one.go", one)
	single, err := measure.Tree(root)
	if err != nil {
		t.Fatal(err)
	}
	none, err := measure.Tree(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	text, rows := table(t, []string{"a", "longname"}, single, none)
	first := strings.Split(text, "\n")[count.First]
	if !strings.HasSuffix(first, "      a  longname") {
		t.Errorf("header %q", first)
	}
	files, commas := rows[count.One], rows[count.Pair+count.One]
	if !slices.Equal(files, []string{"files", "1", "0"}) {
		t.Errorf("files row %q", files)
	}
	want := []string{"commas", "a", "sentence", "1.00", "0.00"}
	if !slices.Equal(commas, want) {
		t.Errorf("commas row %q", commas)
	}
}

// names without a count are refused.
func TestTableRefusesNamesWithoutCounts(t *testing.T) {
	var out bytes.Buffer
	if err := measure.Table(&out, []string{"a"}, nil); err == nil {
		t.Error("a name with no count is an error")
	}
	if out.Len() != count.Nothing {
		t.Errorf("a refused table wrote %q", out.String())
	}
}

// trees are their Go files that are not tests and their Markdown, in
// every directory but .git and testdata.
func TestTree(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.go", "// one sentence here.\npackage a\n")
	write(t, root, "a_test.go", "// not counted.\npackage a\n")
	write(t, root, "pkg/b.go", "// one more. and another.\npackage b\n")
	write(t, root, "README.md", "Four words of prose here.\n")
	write(t, root, "notes.txt", "Not read, not read.\n")
	write(t, root, "testdata/x.go", "// not counted.\npackage x\n")
	write(t, root, ".git/x.go", "// not counted.\npackage x\n")
	tally, err := measure.Tree(root)
	if err != nil {
		t.Fatal(err)
	}
	_, rows := table(t, []string{"t"}, tally)
	files, sentences := rows[count.One], rows[count.Pair]
	if !slices.Equal(files, []string{"files", "3"}) ||
		!slices.Equal(sentences, []string{"sentences", "4"}) {
		t.Errorf("rows %q %q", files, sentences)
	}
}

// missing trees are errors.
func TestTreeMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none")
	if _, err := measure.Tree(missing); err == nil {
		t.Error("a missing tree is an error")
	}
}

// unreadable files are errors.
func TestTreeUnreadableFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.go", "// one sentence here.\npackage a\n")
	if err := os.Chmod(filepath.Join(root, "a.go"), noMode); err != nil {
		t.Fatal(err)
	}
	if _, err := measure.Tree(root); err == nil {
		t.Error("a file that cannot be read is an error")
	}
}

// read at a revision, the files are as they were committed and not as
// they are on disk. The mock holds what git would say.
func TestAt(t *testing.T) {
	repo := NewMockRepo(gomock.NewController(t))
	repo.EXPECT().List("root", "HEAD").Return(
		[]string{"a.go", "b.go", "notes.txt"}, nil)
	repo.EXPECT().Show("root", "HEAD", "a.go").
		Return([]byte("// one. two.\npackage a\n"), nil)
	repo.EXPECT().Show("root", "HEAD", "b.go").
		Return([]byte("// three.\npackage a\n"), nil)
	tally, err := measure.At(repo, "root", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	_, rows := table(t, []string{"t"}, tally)
	files, sentences := rows[count.One], rows[count.Pair]
	if !slices.Equal(files, []string{"files", "2"}) ||
		!slices.Equal(sentences, []string{"sentences", "3"}) {
		t.Errorf("rows %q %q", files, sentences)
	}
}

// revisions git does not know are errors, and nothing is shown.
func TestAtRevisionGitDoesNotKnow(t *testing.T) {
	repo := NewMockRepo(gomock.NewController(t))
	refusal := errors.New("unknown revision")
	repo.EXPECT().List("root", "nothing").Return(nil, refusal)
	if _, err := measure.At(repo, "root", "nothing"); !errors.Is(err, refusal) {
		t.Errorf("a revision git does not know: %v", err)
	}
}

// files git cannot show are errors, and the files after them are not
// asked for.
func TestAtFileGitCannotShow(t *testing.T) {
	repo := NewMockRepo(gomock.NewController(t))
	refusal := errors.New("cannot show")
	repo.EXPECT().List("root", "HEAD").Return([]string{"a.go", "b.go"}, nil)
	repo.EXPECT().Show("root", "HEAD", "a.go").Return(nil, refusal)
	if _, err := measure.At(repo, "root", "HEAD"); !errors.Is(err, refusal) {
		t.Errorf("a file git cannot show: %v", err)
	}
}

// gitIn runs git in dir for a test, without signing and as nobody.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	quiet := []string{"-c", "commit.gpgsign=false", "-c", "user.name=t",
		"-c", "user.email=t@t"}
	cmd := exec.Command("git", append(quiet, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
}

// the real git lists and shows the files as they were committed.
func TestGitListsAndShowsCommittedFiles(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.go", "// one. two.\npackage a\n")
	write(t, root, "pkg/b.go", "// three.\npackage a\n")
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "a")
	write(t, root, "a.go", "// changed on disk.\npackage a\n")
	paths, err := measure.Git().List(root, "HEAD")
	if err != nil || !slices.Equal(paths, []string{"a.go", "pkg/b.go"}) {
		t.Fatalf("listed %q, %v", paths, err)
	}
	text, err := measure.Git().Show(root, "HEAD", "a.go")
	if err != nil || string(text) != "// one. two.\npackage a\n" {
		t.Errorf("showed %q, %v", text, err)
	}
	tally, err := measure.At(measure.Git(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	_, rows := table(t, []string{"t"}, tally)
	if !slices.Equal(rows[count.Pair], []string{"sentences", "3"}) {
		t.Errorf("rows %q", rows[count.Pair])
	}
}

// commits with no files list none.
func TestGitListsNothingForAnEmptyCommit(t *testing.T) {
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "commit", "-q", "--allow-empty", "-m", "empty")
	paths, err := measure.Git().List(root, "HEAD")
	if err != nil || len(paths) != count.Nothing {
		t.Errorf("listed %q, %v", paths, err)
	}
}

// git says what it could not do, in its own words.
func TestGitRefusesWhatItDoesNotKnow(t *testing.T) {
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	if _, err := measure.Git().List(root, "nothing"); err == nil ||
		!strings.HasPrefix(err.Error(), "git ls-tree") {
		t.Errorf("an unknown revision: %v", err)
	}
	none := filepath.Join(root, "none")
	if _, err := measure.Git().Show(none, "HEAD", "a.go"); err == nil {
		t.Error("a missing tree is an error")
	}
}

// write puts a file under root, making its directories.
func write(t *testing.T, root, name, text string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), fileMode); err != nil {
		t.Fatal(err)
	}
}
