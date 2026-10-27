package voice_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/voice"
	"pgregory.net/rapid"
)

// fileMode is the mode of a file a test writes, defined by unix.
const fileMode = 0o644

// two is a shots file with two examples, one indented by a tab.
const two = `== before
// run runs. it also waits.
func run() {
== after
// run runs.
== before
	// here, because there.
	if x {
== after
	// here.

`

// load is a voice from rules and shots text, through a directory.
func load(t *testing.T, rules, shots string) (voice.Voice, error) {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "rules.txt", rules)
	write(t, dir, "shots.txt", shots)
	return voice.Load(dir)
}

// the shots keep their text as it is, less the trailing newlines.
func TestShots(t *testing.T) {
	loaded, err := load(t, "be plain", two)
	if err != nil {
		t.Fatal(err)
	}
	shots := loaded.Shots()
	if len(shots) != count.Pair {
		t.Fatalf("got %d shots", len(shots))
	}
	first, second := shots[count.First], shots[count.One]
	if first.Before() != "// run runs. it also waits.\nfunc run() {" ||
		first.After() != "// run runs." {
		t.Errorf("first: %q / %q", first.Before(), first.After())
	}
	if second.Before() != "\t// here, because there.\n\tif x {" ||
		second.After() != "\t// here." {
		t.Errorf("second: %q / %q", second.Before(), second.After())
	}
}

// shots files with nothing in them have no shots.
func TestBlankShots(t *testing.T) {
	none, err := load(t, "be plain", "\n\n")
	if err != nil || len(none.Shots()) != count.Nothing {
		t.Errorf("blank shots: %v %v", none.Shots(), err)
	}
}

// each way a shots file can be wrong is said.
func TestShotsErrors(t *testing.T) {
	cases := map[string]string{
		"text before the first": "stray\n== before\nx\n== after\ny\n",
		"a before without":      "== before\nx\n",
		"example 1 is not":      "== after\ny\n== before\nx\n",
		"example 2 is not": "== before\nx\n== after\ny\n== after\nz\n" +
			"== after\nw\n",
	}
	for want, text := range cases {
		_, err := load(t, "be plain", text)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("shots %q: %v, want %q", text, err, want)
		}
	}
}

// the built-in voice has rules and a shot for each example in its file.
func TestDefault(t *testing.T) {
	built, err := voice.Default()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(built.Rules(), "comment lines only") {
		t.Errorf("rules: %q", built.Rules())
	}
	text, err := os.ReadFile("shots.txt")
	if err != nil {
		t.Fatal(err)
	}
	examples := strings.Count(string(text), "\n== before\n")
	shots := built.Shots()
	if len(shots) != examples+count.One || len(shots) == count.Nothing {
		t.Errorf("%d shots for %d examples", len(shots), examples+count.One)
	}
}

// voices load from a directory and are trimmed. Loads say what is
// missing or blank.
func TestLoad(t *testing.T) {
	loaded, err := load(t, "\nbe plain\n", two)
	if err != nil {
		t.Fatal(err)
	}
	shots := loaded.Shots()
	if loaded.Rules() != "be plain" || len(shots) != count.Pair {
		t.Errorf("loaded %q with %d shots", loaded.Rules(), len(shots))
	}
}

// empty directories are errors.
func TestLoadEmptyDirectory(t *testing.T) {
	if _, err := voice.Load(t.TempDir()); err == nil {
		t.Error("an empty directory is an error")
	}
}

// shots without rules are not a voice.
func TestLoadShotsWithoutRules(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "shots.txt", two)
	if _, err := voice.Load(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("shots without rules: %v", err)
	}
}

// rules without shots are not a voice.
func TestLoadRulesWithoutShots(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "rules.txt", "be plain")
	if _, err := voice.Load(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("rules without shots: %v", err)
	}
}

// blank rules are said to be blank.
func TestLoadBlankRules(t *testing.T) {
	if _, err := load(t, "  \n", two); err == nil ||
		!strings.Contains(err.Error(), "blank") {
		t.Errorf("blank rules: %v", err)
	}
}

// directories that are not there are errors.
func TestLoadMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none")
	if _, err := voice.Load(missing); err == nil {
		t.Error("a missing directory is an error")
	}
}

// write puts a file in dir.
func write(t *testing.T, dir, name, text string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(text), fileMode); err != nil {
		t.Fatal(err)
	}
}

// someLines is whether a side of a shot has any lines.
func someLines(lines []string) bool { return len(lines) > count.Nothing }

// drawBody draws the text of one side of a shot: some lines.
func drawBody(check *rapid.T, name string) string {
	line := rapid.StringMatching(`[a-z /\t]{1,12}`)
	lines := rapid.SliceOf(line).Filter(someLines).Draw(check, name)
	return strings.Join(lines, "\n")
}

// roundTrips is the property of TestShotsRoundTrip: shots written in
// the file's form read back the same.
type roundTrips struct{ t *testing.T }

// check draws shots and writes them. Then it reads them back and
// compares.
func (trip roundTrips) check(check *rapid.T) {
	shots := rapid.IntRange(count.Nothing, count.Pair+count.Pair).
		Draw(check, "shots")
	var text strings.Builder
	var befores, afters []string
	for range shots {
		before := drawBody(check, "before")
		after := drawBody(check, "after")
		befores = append(befores, before)
		afters = append(afters, after)
		text.WriteString("== before\n" + before + "\n")
		text.WriteString("== after\n" + after + "\n")
	}
	dir := trip.t.TempDir()
	write(trip.t, dir, "rules.txt", "be plain")
	write(trip.t, dir, "shots.txt", text.String())
	loaded, err := voice.Load(dir)
	read := loaded.Shots()
	if err != nil || len(read) != shots {
		check.Fatalf("%d shots, %v, from %q", len(read), err, text.String())
	}
	for place, shot := range read {
		if shot.Before() != befores[place] || shot.After() != afters[place] {
			check.Fatalf("shot %d read back %q / %q, want %q / %q", place,
				shot.Before(), shot.After(), befores[place], afters[place])
		}
	}
}

// any shots written in the file's form read back the same.
func TestShotsRoundTrip(t *testing.T) {
	rapid.Check(t, roundTrips{t}.check)
}
