package draft_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/okapi/internal/comment"
	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/draft"
	"github.com/janearc/okapi/internal/ollama"
	"github.com/janearc/okapi/internal/picked"
	"github.com/janearc/okapi/internal/voice"
	"go.uber.org/mock/gomock"
)

// fileMode is the mode of a file a test writes, defined by unix.
const fileMode = 0o644

// builtin is the built-in voice, or the test fails.
func builtin(t *testing.T) voice.Voice {
	t.Helper()
	author, err := voice.Default()
	if err != nil {
		t.Fatal(err)
	}
	return author
}

// blocksOf is the comment blocks of a Go file's text, named a.go.
func blocksOf(text string) []comment.Block {
	return comment.Blocks("a.go", text)
}

// the model is told the rules and shown every shot. Then it is given
// the block with its code. Only the comment lines of its answer are
// kept, trimmed on the right.
func TestRunAsks(t *testing.T) {
	author := builtin(t)
	blocks := blocksOf("// old, old, old.\nfunc old() {\n")
	messages := []ollama.Message{ollama.System(author.Rules())}
	for _, shot := range author.Shots() {
		shown := ollama.User(shot.Before())
		said := ollama.Assistant(shot.After())
		messages = append(messages, shown, said)
	}
	asking := ollama.User("// old, old, old.\nfunc old() {")
	messages = append(messages, asking)
	model := NewMockChatter(gomock.NewController(t))
	model.EXPECT().Chat(messages).Return(
		"Sure:\n// plain  \nnot this\n\t// two\r\n\n /c\n", nil)
	done, err := draft.Run(model, author, blocks, &bytes.Buffer{})
	if err != nil || len(done) != count.One ||
		done[count.First].After() != "// plain\n\t// two" {
		t.Fatalf("got %v, %v", done, err)
	}
}

// progress is told every so many blocks; on a failure the blocks
// drafted so far come back with an error that names the one that
// failed.
func TestRunProgress(t *testing.T) {
	every := picked.Whole("progress-every")
	failsAt := every * count.Pair
	total := failsAt + count.One
	model := NewMockChatter(gomock.NewController(t))
	model.EXPECT().Chat(gomock.Any()).Return("// plain", nil).Times(failsAt)
	model.EXPECT().Chat(gomock.Any()).Return("", errors.New("tired"))
	blocks := blocksOf(strings.Repeat("// x\n// y\nvar z\n", total))
	if len(blocks) != total {
		t.Fatalf("%d blocks", len(blocks))
	}
	var out bytes.Buffer
	done, err := draft.Run(model, builtin(t), blocks, &out)
	if err == nil || !strings.HasPrefix(err.Error(), blocks[failsAt].Key()+": ") {
		t.Errorf("err %v", err)
	}
	if len(done) != failsAt || done[failsAt-count.One].After() != "// plain" ||
		done[count.First].Key() != "a.go:1" {
		t.Errorf("done %d", len(done))
	}
	said := strings.TrimSpace(out.String())
	lines := strings.Split(said, "\n")
	first := fmt.Sprintf("%d/%d in ", every, total)
	second := fmt.Sprintf("%d/%d in ", failsAt, total)
	if len(lines) != count.Pair || !strings.HasPrefix(lines[count.First], first) ||
		!strings.HasPrefix(lines[count.One], second) {
		t.Errorf("progress %q", out.String())
	}
}

// runs that do not fail return every block and tell no progress
// before the first so many.
func TestRunAllBlocks(t *testing.T) {
	model := NewMockChatter(gomock.NewController(t))
	model.EXPECT().Chat(gomock.Any()).Return("// plain", nil).Times(count.Pair)
	blocks := blocksOf(strings.Repeat("// x\n// y\nvar z\n", count.Pair))
	var out bytes.Buffer
	done, err := draft.Run(model, builtin(t), blocks, &out)
	if err != nil || len(done) != count.Pair || out.Len() != count.Nothing {
		t.Errorf("done %d, %v, said %q", len(done), err, out.String())
	}
}

// only blocks with a reason are drafted.
func TestFlagged(t *testing.T) {
	blocks := blocksOf("// plain.\nvar a\n// one\n// two\nvar b\n" +
		"// plain.\n")
	got := draft.Flagged(blocks)
	if len(got) != count.One || got[count.First].Key() != "a.go:3" {
		t.Errorf("got %v of %v", got, blocks)
	}
}

// drafts files read back as they were written, and bad ones say which
// file.
func TestWriteRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.json")
	blocks := blocksOf("// x\nvar x\n")
	blocks[count.First].Drafted("// y")
	if err := draft.Write(path, blocks); err != nil {
		t.Fatal(err)
	}
	back, err := draft.Read(path)
	if err != nil || len(back) != count.One {
		t.Fatalf("read back %v, %v", back, err)
	}
	block := back[count.First]
	if block.After() != "// y" || block.Code() != "var x" ||
		block.Key() != "a.go:1" {
		t.Fatalf("read back %v", block)
	}
}

// missing files are not-exist, and empty lists read back empty.
func TestReadMissingAndEmpty(t *testing.T) {
	dir := t.TempDir()
	none := filepath.Join(dir, "none")
	if _, err := draft.Read(none); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file: %v", err)
	}
	path := filepath.Join(dir, "drafts.json")
	if err := draft.Write(path, nil); err != nil {
		t.Fatal(err)
	}
	if back, err := draft.Read(path); err != nil || len(back) != count.Nothing {
		t.Errorf("empty: %v, %v", back, err)
	}
}

// drafts files that are not json say which file, and paths under files
// cannot be written.
func TestReadBadAndWriteUnder(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{"), fileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Read(bad); err == nil ||
		!strings.Contains(err.Error(), "bad.json") {
		t.Errorf("bad json: %v", err)
	}
	under := filepath.Join(bad, "x")
	if err := draft.Write(under, nil); err == nil {
		t.Error("a path under a file is an error")
	}
}
