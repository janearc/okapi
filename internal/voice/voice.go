// Package voice is how the author writes a comment: the rules the
// model is told, and the before-and-after pairs it is shown.
package voice

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/janearc/okapi/internal/count"
)

//go:embed rules.txt shots.txt
var builtin embed.FS

// Shot is one example: a comment as it was, and as the author made it.
type Shot struct{ before, after string }

// Before is the comment as it was, with the code under it.
func (shot Shot) Before() string { return shot.before }

// After is the comment as the author made it.
func (shot Shot) After() string { return shot.after }

// Voice is the rules and the shots.
type Voice struct {
	rules string
	shots []Shot
}

// Rules is what the model is told before it sees any example.
func (voice Voice) Rules() string { return voice.rules }

// Shots are the examples, in the order they are shown.
func (voice Voice) Shots() []Shot { return voice.shots }

// Default is the voice built into okapi.
func Default() (Voice, error) {
	// The files are embedded, so they are always there.
	rules, _ := builtin.ReadFile("rules.txt")
	shots, _ := builtin.ReadFile("shots.txt")
	return parse(string(rules), string(shots))
}

// Load reads rules.txt and shots.txt from dir.
func Load(dir string) (Voice, error) {
	rules, err := os.ReadFile(filepath.Join(dir, "rules.txt"))
	if err != nil {
		return Voice{}, err
	}
	shots, err := os.ReadFile(filepath.Join(dir, "shots.txt"))
	if err != nil {
		return Voice{}, err
	}
	return parse(string(rules), string(shots))
}

// parse is a voice from the text of its two files.
func parse(rules, shots string) (Voice, error) {
	if strings.TrimSpace(rules) == "" {
		return Voice{}, fmt.Errorf("the rules are blank")
	}
	parsed, err := parseShots(shots)
	if err != nil {
		return Voice{}, err
	}
	return Voice{rules: strings.TrimSpace(rules), shots: parsed}, nil
}

// errStray is a shots file with text before its first example.
var errStray = errors.New("shots: text before the first == before")

// errUnpaired is a shots file whose last before has no after.
var errUnpaired = errors.New("shots: a before without its after")

// unpaired is the error for an example that is not a before and then
// an after. Examples are numbered from one.
const unpaired = "shots: example %d is not a before, then an after"

// parseShots reads shots from their text. The line "== before" opens
// an example. The line "== after" gives its answer. The text between
// such lines is kept as it is, less its trailing newlines. Every
// before needs an after.
func parseShots(text string) ([]Shot, error) {
	var kinds, bodies []string
	for _, line := range strings.Split(text, "\n") {
		if line == "== before" || line == "== after" {
			kinds = append(kinds, strings.TrimPrefix(line, "== "))
			bodies = append(bodies, "")
			continue
		}
		if len(kinds) == count.Nothing {
			if strings.TrimSpace(line) != "" {
				return nil, errStray
			}
			continue
		}
		bodies[len(bodies)-count.One] += line + "\n"
	}
	return pair(kinds, bodies)
}

// pair makes shots of the sections in order: a before, then its after.
func pair(kinds, bodies []string) ([]Shot, error) {
	if len(kinds)%count.Pair != count.Nothing {
		return nil, errUnpaired
	}
	var out []Shot
	for i := count.First; i < len(kinds); i += count.Pair {
		if kinds[i] != "before" || kinds[i+count.One] != "after" {
			return nil, fmt.Errorf(unpaired, i/count.Pair+count.One)
		}
		out = append(out, Shot{
			before: strings.TrimRight(bodies[i], "\n"),
			after:  strings.TrimRight(bodies[i+count.One], "\n"),
		})
	}
	return out, nil
}
