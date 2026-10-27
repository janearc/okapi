// Package draft asks the model for a plain version of each comment
// block and keeps the answers beside the originals.
package draft

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/janearc/okapi/internal/comment"
	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/ollama"
	"github.com/janearc/okapi/internal/picked"
	"github.com/janearc/okapi/internal/voice"
)

// fileMode is the mode of a new drafts file, defined by unix. The
// owner reads and writes it and everyone else reads it.
const fileMode = 0o644

// Chatter answers one chat: the messages in, the model's answer out.
type Chatter interface {
	// Chat sends the messages and returns the answer.
	Chat(messages []ollama.Message) (string, error)
}

// isComment matches a line that is a // comment, indented or not.
var isComment = regexp.MustCompile(`^\s*//`)

// Run drafts every block in turn, writing a line of progress on out
// every twenty blocks. On an error it returns the blocks drafted so
// far with the error, which names the block that failed.
func Run(
	client Chatter, author voice.Voice, blocks []comment.Block,
	out io.Writer,
) ([]comment.Block, error) {
	start := time.Now()
	done := make([]comment.Block, count.Nothing, len(blocks))
	for _, block := range blocks {
		text, err := ask(client, author, block)
		if err != nil {
			return done, fmt.Errorf("%s: %w", block.Key(), err)
		}
		block.Drafted(text)
		done = append(done, block)
		if len(done)%picked.Whole("progress-every") == count.Nothing {
			took := time.Since(start).Seconds()
			fmt.Fprintf(out, "%d/%d in %.0fs\n",
				len(done), len(blocks), took)
		}
	}
	return done, nil
}

// ask is one block's draft. The model sees the author's rules and then
// the shots and then the block with its code. Only the comment lines of
// the answer are kept.
func ask(
	client Chatter, author voice.Voice, block comment.Block,
) (string, error) {
	messages := []ollama.Message{ollama.System(author.Rules())}
	for _, shot := range author.Shots() {
		shown := ollama.User(shot.Before())
		said := ollama.Assistant(shot.After())
		messages = append(messages, shown, said)
	}
	asking := ollama.User(block.Before() + "\n" + block.Code())
	messages = append(messages, asking)
	reply, err := client.Chat(messages)
	if err != nil {
		return "", err
	}
	return commentLines(reply), nil
}

// commentLines keeps the lines of text that are // comments and trims
// their right ends. It drops the rest.
func commentLines(text string) string {
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if isComment.MatchString(line) {
			kept = append(kept, strings.TrimRight(line, " \t\r"))
		}
	}
	return strings.Join(kept, "\n")
}

// Flagged is the blocks worth a draft: those with a reason.
func Flagged(blocks []comment.Block) []comment.Block {
	var out []comment.Block
	for _, block := range blocks {
		if len(block.Reasons()) > count.Nothing {
			out = append(out, block)
		}
	}
	return out
}

// Write writes the blocks to path as JSON, for a reviewer and for
// apply.
func Write(path string, blocks []comment.Block) error {
	// Blocks marshal to strings and a number, so this cannot fail.
	data, _ := json.MarshalIndent(blocks, "", " ")
	return os.WriteFile(path, append(data, '\n'), fileMode)
}

// Read reads a drafts file.
func Read(path string) ([]comment.Block, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var blocks []comment.Block
	if err := json.Unmarshal(data, &blocks); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return blocks, nil
}
