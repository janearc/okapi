package main

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/janearc/okapi/internal/apply"
	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/measure"
	"github.com/janearc/puzzlemouth/pzm"
	pv "github.com/janearc/puzzlemouth/verb"
)

// The stamps game writes into the binary when it builds. Plain go
// build leaves them as they are here, and --age says so.
var (
	build       = "dev"
	built       = ""
	stampedWho  = ""
	stampedSids = ""
)

// okapiVerb is one verb as okapi declares it to pzm.
type okapiVerb struct {
	name  pv.Name
	help  pv.Help
	page  pv.Page
	facts pv.Facts
	do    pzm.Do
	flags []pzm.Flag
}

// okapi is the tool: the verbs and what they ask of the machine.
type okapi struct {
	builds buildSource
	models modelSource
	files  apply.Files
	repo   measure.Repo
}

// readOnly gives the facts of a verb that only reads.
func readOnly() pv.Facts {
	return pv.NewFacts(pv.WritesNothing, pv.LeavesAlone, pv.ThisMachine,
		pv.NoOne, pv.Light)
}

// verbs is every verb okapi has, declared once.
func (tool okapi) verbs() []okapiVerb {
	return []okapiVerb{
		tool.measureDecl(), blocksDecl(), tool.draftDecl(), briefDecl(),
		tool.applyDecl(), tool.versionDecl(),
	}
}

// measureDecl declares measure.
func (tool okapi) measureDecl() okapiVerb {
	at := pzm.Value("at", pv.NoShort,
		"count the files at this git revision, not on disk")
	return okapiVerb{name: "measure",
		help: "counts the habits of comments over trees",
		page: measurePage, facts: readOnly(), do: tool.measureVerb,
		flags: []pzm.Flag{at}}
}

// blocksDecl declares blocks.
func blocksDecl() okapiVerb {
	all := pzm.Switch("all", pv.NoShort, "show blocks that are only long")
	history := pzm.Value("history", pv.NoShort,
		"more words that mark history, comma separated")
	return okapiVerb{name: "blocks",
		help: "shows the comment blocks worth a rewrite",
		page: blocksPage, facts: readOnly(), do: blocksVerb,
		flags: []pzm.Flag{all, history}}
}

// draftDecl declares draft. It writes the drafts file and sends comments
// to the ollama endpoint, which OKAPI_OLLAMA can put on another machine.
func (tool okapi) draftDecl() okapiVerb {
	facts := pv.NewFacts(pv.WritesTo("the drafts file"), pv.Changes,
		pv.Outside, pv.NoOne, pv.FullGPU)
	voice := pzm.Value("voice", pv.NoShort,
		"a directory with rules.txt and shots.txt")
	model := pzm.Value("model", pv.NoShort,
		"the model to ask, instead of OKAPI_MODEL")
	return okapiVerb{name: "draft",
		help: "has an ollama model draft plainer comments",
		page: draftPage, facts: facts, do: tool.draftVerb,
		flags: []pzm.Flag{voice, model}}
}

// briefDecl declares brief.
func briefDecl() okapiVerb {
	return okapiVerb{name: "brief", help: "prints what a reviewer is told",
		page: briefPage, facts: readOnly(), do: briefVerb}
}

// applyDecl declares apply. It rewrites the files the drafts name.
func (tool okapi) applyDecl() okapiVerb {
	facts := pv.NewFacts(pv.WritesTree, pv.Changes, pv.ThisMachine,
		pv.NoOne, pv.Light)
	repo := pzm.Value("repo", pv.NoShort,
		"the tree the drafts' paths are under, default .")
	return okapiVerb{name: "apply",
		help: "rewrites the files from a reviewer's verdicts",
		page: applyPage, facts: facts, do: tool.applyVerb,
		flags: []pzm.Flag{repo}}
}

// versionDecl declares version.
func (tool okapi) versionDecl() okapiVerb {
	return okapiVerb{name: "version", help: "says which okapi this is",
		page: versionPage, facts: readOnly(), do: tool.versionVerb}
}

// pzmTool declares okapi and every verb to pzm. Each verb's example is
// the function Example_ and its name in this package's tests.
func (tool okapi) pzmTool() *pzm.Tool {
	declared := pzm.New("okapi", "makes the comments in a Go tree plain",
		pzm.Build(build), pzm.Built(built), pzm.Who(stampedWho),
		pzm.Sessions(stampedSids))
	for _, each := range tool.verbs() {
		example := pv.Example("Example_" + string(each.name))
		declared.Verb(each.name, pv.NoShort, each.help, each.page,
			example, each.facts, each.do, each.flags...)
	}
	return declared
}

// legacyFlags are the flags the old parser took with one hyphen as well
// as two. pzm takes two.
var legacyFlags = []string{"at", "all", "history", "voice", "model", "repo"}

// verbNames are the words the old dispatch took with any hyphens
// before them: the verbs, and help.
func (tool okapi) verbNames() []string {
	names := []string{"help"}
	for _, each := range tool.verbs() {
		names = append(names, string(each.name))
	}
	return names
}

// normalize keeps the spellings the old parser took. The first word
// may have hyphens before a verb, and help means --help. One hyphen is
// read as two on a flag. Words after a lone -- stay as they are.
func (tool okapi) normalize(args []string) []string {
	out := slices.Clone(args)
	if len(out) == count.Nothing {
		return out
	}
	switch first := strings.TrimLeft(out[count.First], "-"); {
	case first == "help":
		return []string{"--help"}
	case slices.Contains(tool.verbNames(), first):
		out[count.First] = first
	}
	for at := count.One; at < len(out) && out[at] != "--"; at++ {
		word := strings.TrimPrefix(out[at], "-")
		name, _, _ := strings.Cut(word, "=")
		single := strings.HasPrefix(out[at], "-") &&
			!strings.HasPrefix(out[at], "--")
		if single && slices.Contains(legacyFlags, name) {
			out[at] = "-" + out[at]
		}
	}
	return out
}

// run runs okapi through pzm with its input and output given. With no
// words it prints the help on errs and exits 2.
func (tool okapi) run(args []string, in io.Reader, out, errs io.Writer) int {
	declared := tool.pzmTool()
	if len(args) == count.Nothing {
		declared.RunWith([]string{"--help"}, in, errs, errs)
		return exitUsage
	}
	return declared.RunWith(tool.normalize(args), in, out, errs)
}

// runPzm runs the real okapi with its input and output given.
func runPzm(args []string, in io.Reader, out, errs io.Writer) int {
	actual := okapi{builds: runtimeBuilds{}, models: ollamaModels{},
		files: apply.Disk(), repo: measure.Git()}
	return actual.run(args, in, out, errs)
}

// need is an error when a verb was given fewer words than it needs,
// telling its shape.
func need(call pzm.Call, least int, shape string) error {
	if len(call.Args()) >= least {
		return nil
	}
	return fmt.Errorf("needs more: %s", shape)
}
