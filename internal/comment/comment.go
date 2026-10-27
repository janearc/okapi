// Package comment reads the // comments of a Go file as blocks. Each
// block is a run of comment lines with the line it starts on and the
// code under it. The package splits a block into sentences and says
// which blocks are worth a rewrite.
package comment

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/picked"
)

// everyMatch asks a regexp for all its matches, defined by regexp: a
// negative count.
const everyMatch = -1

// isComment matches a line that is a // comment, indented or not.
var isComment = regexp.MustCompile(`^\s*//`)

// directive matches a comment line that instructs a tool rather than
// a reader. It never starts a block, and it is kept when the block
// around it is rewritten.
var directive = regexp.MustCompile(
	`^\s*//(go:|\s*game: allow|line |nolint|ninja:)`,
)

// tabbed matches a comment line with a tab after the slashes: a table
// or a sample of code, which is left as it is.
var tabbed = regexp.MustCompile(`^\s*//\t`)

// exempt matches a line that asks to be left alone, by the word noqa.
var exempt = regexp.MustCompile(`\bnoqa\b`)

// marker matches the slashes that open a comment line and the one
// space after them.
var marker = regexp.MustCompile(`^\s*// ?`)

// joined matches a comma and a joining word: one more clause hung on
// the sentence.
var joined = regexp.MustCompile(
	`,\s+(?i:because|so|since|which|and|but|while|then|or|as|where|` +
		`when|unless|though)\b`,
)

// opener matches a sentence that starts with a or an.
var opener = regexp.MustCompile(`^(?i:a|an)\s`)

// Block is one run of comment lines in a file. It holds the code under
// the run and the draft of it once one is made.
type Block struct {
	file   string
	line   int
	before []string
	code   []string
	after  string
}

// File is the path the block was read from.
func (block Block) File() string { return block.file }

// Line is the line of the block's first comment line, counting from 1.
func (block Block) Line() int { return block.line }

// Lines is the comment's lines as they stand in the file.
func (block Block) Lines() []string { return slices.Clone(block.before) }

// Before is the comment as one string, its lines joined by newlines.
func (block Block) Before() string {
	return strings.Join(block.before, "\n")
}

// Code is up to three lines under the comment as one string.
func (block Block) Code() string { return strings.Join(block.code, "\n") }

// After is the draft, or empty when none has been made.
func (block Block) After() string { return block.after }

// Drafted records a draft of the comment.
func (block *Block) Drafted(text string) { block.after = text }

// Key is the file and line of the block, which is how a reviewer names
// it.
func (block Block) Key() string {
	return block.file + ":" + strconv.Itoa(block.line)
}

// Indent is the whitespace before the slashes on the first line.
func (block Block) Indent() string {
	first := block.before[count.First]
	return first[:len(first)-len(strings.TrimLeft(first, " \t"))]
}

// Directives are the block's lines that instruct a tool.
func (block Block) Directives() []string {
	var out []string
	for _, line := range block.before {
		if IsDirective(line) {
			out = append(out, line)
		}
	}
	return out
}

// Sentences are the block's prose, split into sentences.
func (block Block) Sentences() []string { return sentences(block.before) }

// Reasons says why a block is worth a rewrite. Long means it runs past
// one line. Commas means a sentence holds two or more. Chain means a
// sentence hangs two or more clauses on commas and joining words.
// Opener means a sentence starts with a or an. Empty means leave it.
func (block Block) Reasons() []string {
	var out []string
	if len(block.before) > count.One {
		out = append(out, "long")
	}
	found := block.Sentences()
	tests := []struct {
		name string
		hit  func(string) bool
	}{
		{"commas", twoCommas},
		{"chain", Chains},
		{"opener", Opens},
	}
	for _, test := range tests {
		if slices.ContainsFunc(found, test.hit) {
			out = append(out, test.name)
		}
	}
	return out
}

// shape is the block as a drafts file holds it. It carries the block
// from draft to the reviewer to apply. Its fields are the file's keys.
type shape struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Before string `json:"before"`
	Code   string `json:"code"`
	After  string `json:"after"`
}

// MarshalJSON writes the block as a drafts file holds it.
func (block Block) MarshalJSON() ([]byte, error) {
	return json.Marshal(shape{
		File:   block.file,
		Line:   block.line,
		Before: block.Before(),
		Code:   block.Code(),
		After:  block.after,
	})
}

// UnmarshalJSON reads a block as a drafts file holds it.
func (block *Block) UnmarshalJSON(data []byte) error {
	var read shape
	if err := json.Unmarshal(data, &read); err != nil {
		return err
	}
	*block = Block{
		file:   read.File,
		line:   read.Line,
		before: lines(read.Before),
		code:   lines(read.Code),
		after:  read.After,
	}
	return nil
}

// lines splits text on newlines, with nothing for empty text.
func lines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// IsDirective says whether a comment line instructs a tool.
func IsDirective(line string) bool { return directive.MatchString(line) }

// Blocks is every comment block in text, the contents of the file
// named file. A directive never starts a block. A block with a tab
// after the slashes is a table or a code sample and is left out, and
// so is a block with noqa on any of its lines.
func Blocks(file, text string) []Block {
	all := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	var out []Block
	for i := count.First; i < len(all); {
		if !isComment.MatchString(all[i]) || IsDirective(all[i]) {
			i++
			continue
		}
		end := i
		for end < len(all) && isComment.MatchString(all[end]) {
			end++
		}
		if block, ok := blockAt(file, all, i, end); ok {
			out = append(out, block)
		}
		i = end
	}
	return out
}

// blockAt is the block on lines start to end of all. It holds up to
// three lines of code under it. It is false when a tab follows the
// slashes or the comment asks to be left alone.
func blockAt(file string, all []string, start, end int) (Block, bool) {
	if slices.ContainsFunc(all[start:end], tabbed.MatchString) ||
		slices.ContainsFunc(all[start:end], exempt.MatchString) {
		return Block{}, false
	}
	var code []string
	shown := picked.Whole("code-lines")
	for k := end; k < len(all) && len(code) < shown; k++ {
		if strings.HasPrefix(strings.TrimLeft(all[k], " \t"), "//") {
			break
		}
		code = append(code, all[k])
	}
	return Block{
		file:   file,
		line:   start + count.Origin,
		before: slices.Clone(all[start:end]),
		code:   code,
	}, true
}

// sentences is the prose of comment lines as sentences. The slashes
// are dropped and the lines are joined by one space. Then the text is
// split.
func sentences(commentLines []string) []string {
	parts := make([]string, count.Nothing, len(commentLines))
	for _, line := range commentLines {
		parts = append(parts, marker.ReplaceAllString(line, ""))
	}
	return split(strings.Join(parts, " "))
}

// split cuts text into sentences at a full stop or a question mark
// that has a space after it. Blank pieces are dropped.
func split(text string) []string {
	var out []string
	start := count.First
	for i := count.First; i+count.One < len(text); i++ {
		ends := text[i] == '.' || text[i] == '?'
		if ends && unicode.IsSpace(rune(text[i+count.One])) {
			out = keep(out, text[start:i+count.One])
			start = i + count.One
		}
	}
	return keep(out, text[start:])
}

// keep appends the trimmed piece to out unless it is blank.
func keep(out []string, piece string) []string {
	if piece = strings.TrimSpace(piece); piece != "" {
		out = append(out, piece)
	}
	return out
}

// twoCommas says whether a sentence holds two or more commas.
func twoCommas(sentence string) bool {
	return strings.Count(sentence, ",") >= count.Pair
}

// Chains says whether a sentence hangs two or more clauses on commas
// and joining words.
func Chains(sentence string) bool {
	joins := joined.FindAllStringIndex(sentence, everyMatch)
	return len(joins) >= count.Pair
}

// Opens says whether a sentence starts with a or an.
func Opens(sentence string) bool { return opener.MatchString(sentence) }

// Markdown is the sentences of a Markdown file's prose. Fenced code
// and indented lines are left out. So are headings and table rows. So
// is any sentence of three words or fewer.
func Markdown(text string) []string {
	var kept []string
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if fenced || notProse(line) {
			continue
		}
		kept = append(kept, strings.TrimSpace(line))
	}
	var out []string
	label := picked.Whole("shortest-sentence")
	for _, sentence := range split(strings.Join(kept, " ")) {
		if len(strings.Fields(sentence)) > label {
			out = append(out, sentence)
		}
	}
	return out
}

// notProse says whether a Markdown line is indented, a heading or a
// table row.
func notProse(line string) bool {
	return strings.HasPrefix(line, "    ") ||
		strings.HasPrefix(line, "\t") ||
		strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "|")
}
