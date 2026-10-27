// Package measure counts what a plain comment does without. It reads a
// tree of Go and Markdown files and counts commas, chained clauses and
// sentences that open with a or an.
package measure

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/janearc/okapi/internal/comment"
	"github.com/janearc/okapi/internal/count"
)

// percent is how many parts make a whole share, defined so.
const percent = 100.0

// Count is the tally for one tree.
type Count struct {
	files, sentences, commas, twoCommas, openers, chains int
}

// add tallies one file's sentences.
func (tally *Count) add(sentences []string) {
	tally.files++
	for _, sentence := range sentences {
		tally.sentences++
		commas := strings.Count(sentence, ",")
		tally.commas += commas
		if commas >= count.Pair {
			tally.twoCommas++
		}
		if comment.Opens(sentence) {
			tally.openers++
		}
		if comment.Chains(sentence) {
			tally.chains++
		}
	}
}

// labels name the rows of a table, in the order cells gives them.
var labels = []string{
	"files",
	"sentences",
	"commas a sentence",
	"two or more commas",
	"open with a or an",
	"chain clauses",
}

// share is a part of the total as a percentage.
func share(part int, total float64) string {
	return fmt.Sprintf("%.1f%%", percent*float64(part)/total)
}

// cells is the count as a table column, one cell a label.
func (tally Count) cells() []string {
	total := float64(max(tally.sentences, count.One))
	return []string{
		strconv.Itoa(tally.files),
		strconv.Itoa(tally.sentences),
		fmt.Sprintf("%.2f", float64(tally.commas)/total),
		share(tally.twoCommas, total),
		share(tally.openers, total),
		share(tally.chains, total),
	}
}

// widest is the length of the longest of the words.
func widest(words []string) int {
	wide := count.Nothing
	for _, word := range words {
		wide = max(wide, len(word))
	}
	return wide
}

// grid is a table being written. It holds where the table goes, how
// wide the labels are and how wide each column is.
type grid struct {
	out    io.Writer
	label  int
	widths []int
}

// row writes one row: its head, then a cell for each column.
func (rows grid) row(head string, cells []string) {
	fmt.Fprintf(rows.out, "%-*s", rows.label, head)
	for place, cell := range cells {
		fmt.Fprintf(rows.out, "  %*s", rows.widths[place], cell)
	}
	fmt.Fprintln(rows.out)
}

// across is the cell at place in each of the columns.
func across(columns [][]string, place int) []string {
	cells := make([]string, count.Nothing, len(columns))
	for _, column := range columns {
		cells = append(cells, column[place])
	}
	return cells
}

// Table writes the counts side by side under each tree's name. Each
// tree is a column.
func Table(out io.Writer, names []string, counts []Count) error {
	if len(names) != len(counts) {
		return fmt.Errorf(
			"%d names for %d counts", len(names), len(counts),
		)
	}
	rows := grid{out: out, label: widest(labels),
		widths: make([]int, len(counts))}
	columns := make([][]string, len(counts))
	for place, tally := range counts {
		columns[place] = tally.cells()
		named := append([]string{names[place]}, columns[place]...)
		rows.widths[place] = widest(named)
	}
	rows.row("", names)
	for place, each := range labels {
		rows.row(each, across(columns, place))
	}
	return nil
}

// read is one file's sentences by its name. It reads the comments of a
// Go file that is not a test and the prose of a Markdown file. It
// returns false for any other file.
func read(path, text string) ([]string, bool) {
	switch {
	case strings.HasSuffix(path, "_test.go"):
		return nil, false
	case strings.HasSuffix(path, ".go"):
		var out []string
		for _, block := range comment.Blocks(path, text) {
			out = append(out, block.Sentences()...)
		}
		return out, true
	case strings.HasSuffix(path, ".md"):
		return comment.Markdown(text), true
	}
	return nil, false
}

// fetcher gets the contents of a file by its path.
type fetcher interface {
	fetch(path string) ([]byte, error)
}

// onDisk gets files from the disk.
type onDisk struct{}

// fetch reads the file at path.
func (onDisk) fetch(path string) ([]byte, error) { return os.ReadFile(path) }

// atRevision gets files as they were at a revision.
type atRevision struct {
	repo Repo
	root string
	rev  string
}

// fetch shows the file at path at the revision.
func (from atRevision) fetch(path string) ([]byte, error) {
	return from.repo.Show(from.root, from.rev, path)
}

// addFile tallies one file when read knows its kind, fetching it only
// then.
func (tally *Count) addFile(path string, from fetcher) error {
	if _, ok := read(path, ""); !ok {
		return nil
	}
	text, err := from.fetch(path)
	if err != nil {
		return err
	}
	sentences, _ := read(path, string(text))
	tally.add(sentences)
	return nil
}

// skipped names the directories a walk does not enter.
var skipped = map[string]bool{".git": true, "testdata": true}

// walk is a tally of the files under root as a walk finds them.
type walk struct {
	root  string
	tally *Count
}

// visit tallies one path of the walk, or skips the directory.
func (walking walk) visit(path string, entry fs.DirEntry, err error) error {
	if err != nil {
		return err
	}
	if entry.IsDir() {
		if skipped[entry.Name()] && path != walking.root {
			return filepath.SkipDir
		}
		return nil
	}
	return walking.tally.addFile(path, onDisk{})
}

// Tree tallies every Go file that is not a test and every Markdown
// file under root, as they are on disk.
func Tree(root string) (Count, error) {
	var tally Count
	err := filepath.WalkDir(root, walk{root, &tally}.visit)
	return tally, err
}

// Repo reads the files of a git repository as they were at a revision.
type Repo interface {
	// List is the path of every file at rev, below root.
	List(root, rev string) ([]string, error)
	// Show is the contents of the file at path at rev.
	Show(root, rev, path string) ([]byte, error)
}

// At tallies the same files as Tree, as they are at a git revision of
// the repository at root.
func At(repo Repo, root, rev string) (Count, error) {
	var tally Count
	paths, err := repo.List(root, rev)
	if err != nil {
		return tally, err
	}
	from := atRevision{repo: repo, root: root, rev: rev}
	for _, path := range paths {
		if err := tally.addFile(path, from); err != nil {
			return tally, err
		}
	}
	return tally, nil
}

// git is the git on this machine.
type git struct{}

// Git is the git repositories of this machine.
func Git() Repo { return git{} }

// List runs git ls-tree for every path at rev.
func (git) List(root, rev string) ([]string, error) {
	listed, err := run(root, "ls-tree", "-r", "--name-only", rev)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, path := range strings.Split(listed, "\n") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// Show runs git show for the file at path at rev.
func (git) Show(root, rev, path string) ([]byte, error) {
	text, err := run(root, "show", rev+":"+path)
	return []byte(text), err
}

// run runs one git command in dir and returns what it printed.
// Failures carry git's own words.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		said := strings.TrimSpace(string(out))
		asked := strings.Join(args, " ")
		return "", fmt.Errorf("git %s: %s", asked, said)
	}
	return string(out), nil
}
