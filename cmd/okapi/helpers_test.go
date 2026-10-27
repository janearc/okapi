package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/janearc/okapi/internal/comment"
	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/ollama"
	"go.uber.org/mock/gomock"
)

// The modes tests write files and folders with.
const (
	fileMode = 0o644 // defined by unix: the owner writes, everyone reads
	dirMode  = 0o755 // defined by unix: a folder anyone can enter
)

// habit is a Go file with one block that draft sends to the model.
const habit = `package x

// A habit opens this sentence, and it chains on, and on again, until
// nobody is left.
func Habit() {}
`

// rig is okapi with everything outside it mocked: the build
// information, the model and the files apply writes. Reaching for a
// mock with no expectation fails the test.
type rig struct {
	tool   okapi
	builds *MockbuildSource
	models *MockmodelSource
	client *MockmodelClient
	files  *MockFiles
	repo   *MockRepo
}

// newRig is okapi over fresh mocks.
func newRig(t *testing.T) rig {
	t.Helper()
	ctrl := gomock.NewController(t)
	made := rig{
		builds: NewMockbuildSource(ctrl),
		models: NewMockmodelSource(ctrl),
		client: NewMockmodelClient(ctrl),
		files:  NewMockFiles(ctrl),
		repo:   NewMockRepo(ctrl),
	}
	made.tool = okapi{builds: made.builds, models: made.models,
		files: made.files, repo: made.repo}
	return made
}

// call runs okapi as main does, through pzm. It returns the exit code
// and what okapi wrote on each stream.
func (with rig) call(args ...string) (int, string, string) {
	var out, errs bytes.Buffer
	code := with.tool.run(args, strings.NewReader(""), &out, &errs)
	return code, out.String(), errs.String()
}

// leadsWith is a gomock matcher that accepts messages whose first is
// the one it holds.
type leadsWith struct{ first ollama.Message }

// Matches says whether the messages begin with the held one.
func (lead leadsWith) Matches(value any) bool {
	messages, ok := value.([]ollama.Message)
	return ok && len(messages) > count.Nothing &&
		messages[count.First] == lead.first
}

// String says what the matcher accepts.
func (lead leadsWith) String() string { return "messages led by one" }

// showsExamples is a gomock matcher that accepts messages with more
// than the rules and the block: the voice showed examples.
type showsExamples struct{}

// Matches says whether the messages hold examples.
func (showsExamples) Matches(value any) bool {
	messages, ok := value.([]ollama.Message)
	return ok && len(messages) > count.Pair
}

// String says what the matcher accepts.
func (showsExamples) String() string { return "messages with examples" }

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

// habitTree is a directory holding habit as x.go, with the test run
// from inside it so the drafts name x.go.
func habitTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	if err := os.WriteFile(path, []byte(habit), fileMode); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

// committed is the one file the mock repository holds at HEAD: two
// sentences.
const committed = "package p\n\n// Plain says hello. It stays short.\nfunc P() {}\n"

// onDisk is a tree whose one file holds more than committed does: four
// sentences.
func onDisk(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	text := committed + "\n// More follows. And more.\nfunc Q() {}\n"
	path := filepath.Join(dir, "p.go")
	if err := os.WriteFile(path, []byte(text), fileMode); err != nil {
		t.Fatal(err)
	}
	return dir
}

// mustTime parses an RFC 3339 time or fails the test.
func mustTime(t *testing.T, text string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

// firstLine is the text up to its first newline.
func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}

// endsWith is a gomock matcher that accepts messages whose last is the
// one it holds.
type endsWith struct{ last ollama.Message }

// Matches says whether the messages end with the held one.
func (end endsWith) Matches(value any) bool {
	messages, ok := value.([]ollama.Message)
	return ok && len(messages) > count.Nothing &&
		messages[len(messages)-count.One] == end.last
}

// String says what the matcher accepts.
func (end endsWith) String() string { return "messages ending in one" }

// lastOf is the message that asks the model about habit's one block.
func lastOf(t *testing.T) ollama.Message {
	t.Helper()
	blocks := comment.Blocks("x.go", habit)
	block := blocks[count.First]
	return ollama.User(block.Before() + "\n" + block.Code())
}
