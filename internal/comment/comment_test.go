package comment_test

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"strconv"

	"github.com/janearc/okapi/internal/comment"
	"github.com/janearc/okapi/internal/count"
	"pgregory.net/rapid"
)

// sample is a Go file with one of every kind of comment block.
const sample = `//go:generate go run gen.go
// Package sample is read by the tests.
package sample

// one says one. it is short.
func one() {}

	// a table follows:
	//	left  right
	// end of table
var table = 1

// first line
// second line
//go:embed files
var files embed.FS

func two() {
	// inside: it waits, because the gate is shut, so it waits.
	wait()
}

// five lines of code follow this.
var a1 = 1
var a2 = 2
var a3 = 3
var a4 = 4
var a5 = 5

// this one, with, commas, is left alone.
// noqa: it is quoted from somewhere.
var quoted = 1
// trailing, with nothing under it
`

// the blocks of sample, with the table and the noqa block left out and
// the directive at the top never starting one.
func TestBlocks(t *testing.T) {
	blocks := comment.Blocks("sample.go", sample)
	want := []struct {
		line       string
		before     []string
		code       string
		directives []string
	}{
		{"2", []string{"// Package sample is read by the tests."},
			"package sample\n", nil},
		{"5", []string{"// one says one. it is short."},
			"func one() {}\n", nil},
		{"13", []string{"// first line", "// second line",
			"//go:embed files"},
			"var files embed.FS\n\nfunc two() {", []string{"//go:embed files"}},
		{"19", []string{"\t// inside: it waits, because the gate is " +
			"shut, so it waits."}, "\twait()\n}\n", nil},
		{"23", []string{"// five lines of code follow this."},
			"var a1 = 1\nvar a2 = 2\nvar a3 = 3", nil},
		{"33", []string{"// trailing, with nothing under it"}, "", nil},
	}
	if len(blocks) != len(want) {
		t.Fatalf("got %d blocks, want %d", len(blocks), len(want))
	}
	for i, block := range blocks {
		line := strconv.Itoa(block.Line())
		if block.File() != "sample.go" || line != want[i].line {
			t.Errorf("block %d is at %s, want line %s",
				i, block.Key(), want[i].line)
		}
		if !slices.Equal(block.Lines(), want[i].before) {
			t.Errorf("block %d lines %q, want %q",
				i, block.Lines(), want[i].before)
		}
		if block.Code() != want[i].code {
			t.Errorf("block %d code %q, want %q",
				i, block.Code(), want[i].code)
		}
		if !slices.Equal(block.Directives(), want[i].directives) {
			t.Errorf("block %d directives %q, want %q", i,
				block.Directives(), want[i].directives)
		}
	}
}

// the indent is the whitespace before the slashes.
func TestIndent(t *testing.T) {
	if got := sampleBlock("inside:").Indent(); got != "\t" {
		t.Errorf("indent %q, want a tab", got)
	}
	if got := sampleBlock("Package sample").Indent(); got != "" {
		t.Errorf("indent %q, want none", got)
	}
}

// sampleBlock is the block of sample whose comment holds the words.
func sampleBlock(words string) comment.Block {
	for _, block := range comment.Blocks("sample.go", sample) {
		if strings.Contains(block.Before(), words) {
			return block
		}
	}
	return comment.Block{}
}

// firstBlock is the first comment block in text.
func firstBlock(text string) comment.Block {
	return comment.Blocks("f.go", text)[count.First]
}

// commented is a block of text's lines, each made a comment line.
func commented(text string) comment.Block {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, "// "+line)
	}
	return firstBlock(strings.Join(lines, "\n"))
}

// sentences end at a stop or a question mark followed by space. The
// slashes go and the lines join with one space.
func TestSentences(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"no end", []string{"no end"}},
		{"a. b? c", []string{"a.", "b?", "c"}},
		{". then", []string{".", "then"}},
		{"? huh", []string{"?", "huh"}},
		{"e.g. this", []string{"e.g.", "this"}},
		{"x.  y", []string{"x.", "y"}},
		{"1.5 is a number.", []string{"1.5 is a number."}},
		{"done.", []string{"done."}},
		{"one.\ntwo.", []string{"one.", "two."}},
		{"one. two\nthree", []string{"one.", "two three"}},
	}
	for _, each := range cases {
		got := commented(each.text).Sentences()
		if !slices.Equal(got, each.want) {
			t.Errorf("sentences of %q = %q, want %q",
				each.text, got, each.want)
		}
	}
	tight := firstBlock("  //one. two\n\t//three")
	if got := tight.Sentences(); !slices.Equal(got, []string{
		"one.", "two three",
	}) {
		t.Errorf("without the space: %q", got)
	}
}

// each habit is named once, and a plain line has no reason.
func TestReasons(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"plain says plain.", nil},
		{"one\ntwo", []string{"long"}},
		{"x, y, and z.", []string{"commas"}},
		{"it waits, because it must, so it does.",
			[]string{"commas", "chain"}},
		{"it ran, so it stopped.", nil},
		{"A law is stated.", []string{"opener"}},
		{"an opener. a second.", []string{"opener"}},
		{"a", nil},
		{"first\nx, y, z. An end, which, because.",
			[]string{"long", "commas", "chain", "opener"}},
	}
	for _, each := range cases {
		got := commented(each.text).Reasons()
		if !slices.Equal(got, each.want) {
			t.Errorf("reasons of %q = %q, want %q",
				each.text, got, each.want)
		}
	}
}

// chains and openers are judged a sentence at a time.
func TestChainsAndOpens(t *testing.T) {
	if comment.Chains("one, because two.") {
		t.Error("one join is not a chain")
	}
	if !comment.Chains("one, because two, which three.") {
		t.Error("two joins are a chain")
	}
	if comment.Chains("one,because two,which three.") {
		t.Error("a join needs its space")
	}
	if !comment.Opens("An apple.") || comment.Opens("Any apple.") {
		t.Error("an opener is the word a or an, whole")
	}
}

// markdown keeps prose only. Fences and headings are left out. So are
// tables and indented lines. So are sentences of three words or fewer.
func TestMarkdown(t *testing.T) {
	text := "# title\n\nThis is a sentence of prose. Short one.\n" +
		"```\ncode, with, commas. and more.\n```\n" +
		"| a | b |\n    indented code is here.\n" +
		"\tmore code is here.\n" +
		"Four words right here. Three words here.\n" +
		"And this is the last sentence here.\n"
	want := []string{
		"This is a sentence of prose.",
		"Four words right here.",
		"And this is the last sentence here.",
	}
	if got := comment.Markdown(text); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// directives are tool lines in any of their spellings.
func TestIsDirective(t *testing.T) {
	yes := []string{"//go:embed x", "//nolint", "  // game: allow x",
		"//line a.go:1", "//ninja:vow gate why"}
	no := []string{"// go is a language", "// nolint is a word", "x"}
	for _, line := range yes {
		if !comment.IsDirective(line) {
			t.Errorf("%q is a directive", line)
		}
	}
	for _, line := range no {
		if comment.IsDirective(line) {
			t.Errorf("%q is not a directive", line)
		}
	}
}

// blocks survive the drafts file under the keys a reviewer reads.
func TestJSON(t *testing.T) {
	block := sampleBlock("first line")
	block.Drafted("// files are embedded")
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]any
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatal(err)
	}
	want := []string{"after", "before", "code", "file", "line"}
	if got := slices.Sorted(maps.Keys(keys)); !slices.Equal(got, want) {
		t.Errorf("keys %q, want %q", got, want)
	}
	var back comment.Block
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Key() != block.Key() || back.Before() != block.Before() ||
		back.Code() != block.Code() || back.After() != block.After() ||
		!slices.Equal(back.Lines(), block.Lines()) {
		t.Errorf("got %+v back, want %+v", back, block)
	}
	var empty comment.Block
	if err := json.Unmarshal([]byte(`{"file":"a.go","line":1,`+
		`"before":"// x","code":"","after":""}`), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Code() != "" || empty.After() != "" {
		t.Errorf("empty code read as %q", empty.Code())
	}
	if err := json.Unmarshal([]byte(`{"before":""}`), &empty); err != nil {
		t.Fatal(err)
	}
	if len(empty.Lines()) != count.Nothing {
		t.Errorf("no before read as %q", empty.Lines())
	}
	if err := json.Unmarshal([]byte(`{"line":"one"}`), &empty); err == nil {
		t.Error("a line that is not a number is an error")
	}
}

// splitting loses no word and adds no space, whatever the text.
func TestSentencesKeepEveryWord(t *testing.T) {
	rapid.Check(t, keepsEveryWord)
}

// keepsEveryWord is the property of TestSentencesKeepEveryWord.
func keepsEveryWord(check *rapid.T) {
	prose := rapid.StringMatching(`([a-z]{1,6}[.?,]?[ \t\n]{0,2}){0,12}`)
	text := prose.Draw(check, "text")
	pieces := commented(text).Sentences()
	got := strings.Fields(strings.Join(pieces, " "))
	if want := strings.Fields(text); !slices.Equal(got, want) {
		check.Fatalf("sentences of %q = %q, lose or add a word", text,
			pieces)
	}
	for _, piece := range pieces {
		if piece == "" || piece != strings.TrimSpace(piece) {
			check.Fatalf("%q gave %q", text, piece)
		}
	}
}
