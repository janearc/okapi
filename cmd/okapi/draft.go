package main

import (
	"fmt"
	"os"

	"github.com/janearc/okapi/internal/comment"
	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/draft"
	"github.com/janearc/okapi/internal/voice"
	"github.com/janearc/puzzlemouth/pzm"
)

// draftPage is what okapi draft says of itself.
const draftPage = `# okapi draft [--voice DIR] [--model NAME] OUT.json FILE ...

Has the model draft a plain version of every comment block worth a
rewrite in the Go files, and writes the drafts to OUT.json beside the
originals. Nothing in the files is changed. The model runs through
ollama, on this machine unless OKAPI_OLLAMA names an endpoint on
another one; then the comments are sent there. OKAPI_OLLAMA names the
endpoint and
OKAPI_MODEL the model when they are not the defaults.

The voice is the rules the model is told and the examples it is shown.
The built-in voice is replaced by a directory holding rules.txt and
shots.txt; okapi's README says their form.

--model NAME asks that model instead of OKAPI_MODEL or the default.

--dry-run says how many blocks would be drafted, and does not ask the
model or write the file.
`

// draftNeeds is how many words draft takes: the drafts file and a Go
// file at least, counted by the words.
const draftNeeds = count.Pair

// draftVerb is okapi draft: the flagged blocks of the files, drafted
// by the model and written as JSON.
func (tool okapi) draftVerb(call pzm.Call) error {
	err := need(call, draftNeeds, "okapi draft OUT.json FILE ...")
	if err != nil {
		return err
	}
	author, err := loadVoice(call.Value("voice"))
	if err != nil {
		return err
	}
	client := tool.models.Client(call.Value("model"))
	if !call.DryRun() && !client.Up() {
		return fmt.Errorf("ollama is not answering at %s", client.URL())
	}
	blocks, err := flaggedIn(call.Args()[count.One:])
	if err != nil {
		return err
	}
	target := call.Args()[count.First]
	if call.DryRun() {
		_, err = fmt.Fprintf(call.Out(),
			"would draft %d blocks with %s to %s\n",
			len(blocks), client.Model(), target)
		return err
	}
	fmt.Fprintf(call.Out(), "%d blocks to draft with %s\n", len(blocks),
		client.Model())
	done, err := draft.Run(client, author, blocks, call.Out())
	if written := draft.Write(target, done); written != nil {
		return written
	}
	if err != nil {
		return fmt.Errorf("%d of %d drafted and written to %s; then %w",
			len(done), len(blocks), target, err)
	}
	fmt.Fprintf(call.Out(), "%d blocks drafted, written to %s\n",
		len(done), target)
	return nil
}

// loadVoice is the built-in voice, or the one in dir when given.
func loadVoice(dir string) (voice.Voice, error) {
	if dir == "" {
		return voice.Default()
	}
	return voice.Load(dir)
}

// flaggedIn is every block worth a draft in the files, in file order.
func flaggedIn(paths []string) ([]comment.Block, error) {
	var out []comment.Block
	for _, path := range paths {
		text, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		blocks := comment.Blocks(path, string(text))
		out = append(out, draft.Flagged(blocks)...)
	}
	return out, nil
}
