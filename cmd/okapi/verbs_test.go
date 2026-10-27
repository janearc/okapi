package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"testing"

	"github.com/janearc/okapi/internal/comment"
	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/draft"
	"github.com/janearc/okapi/internal/ollama"
	"go.uber.org/mock/gomock"
)

// shape is what a verb says when it is given too few arguments.
func shape(verb, usage string) string {
	return "okapi " + verb + ": needs more: " + usage + "\n"
}

// failed is whether a call failed as a verb does. That is exit 1 with
// nothing on standard out and the verb's name first on standard error.
func failed(code int, out, errs, verb string) bool {
	return code == exitFailed && out == "" &&
		strings.HasPrefix(errs, "okapi "+verb+": ")
}

func TestMeasureIsReachedThroughPzmWithItsTrees(t *testing.T) {
	code, out, errs := newRig(t).call("measure", "mine=testdata/tree",
		"again=testdata/tree")
	if code != exitOK || errs != "" {
		t.Fatalf("measure exited %d: %q", code, errs)
	}
	header := strings.Fields(firstLine(out))
	if !slices.Equal(header, []string{"mine", "again"}) {
		t.Fatalf("measure named the trees %q, want mine and again", header)
	}
	if !regexp.MustCompile(`sentences\s+6\s+6\n`).MatchString(out) {
		t.Fatalf("measure did not count the tree:\n%s", out)
	}
}

func TestMeasureNamesABarePathAfterItsDirectory(t *testing.T) {
	_, out, _ := newRig(t).call("measure", "testdata/tree")
	if header := strings.Fields(firstLine(out)); !slices.Equal(header,
		[]string{"tree"}) {
		t.Fatalf("a bare path was named %q, want tree", header)
	}
}

func TestMeasureNamesAnAbsolutePathAfterItsDirectory(t *testing.T) {
	path, err := filepath.Abs("testdata/tree")
	if err != nil {
		t.Fatal(err)
	}
	name, got := named(path)
	if name != "tree" || got != path {
		t.Fatalf("named gave %q, %q", name, got)
	}
}

func TestMeasureAtReadsTheRevisionNotTheDisk(t *testing.T) {
	dir := onDisk(t)
	_, disk, _ := newRig(t).call("measure", "mine="+dir)
	with := newRig(t)
	with.repo.EXPECT().List(dir, "HEAD").Return([]string{"p.go"}, nil)
	with.repo.EXPECT().Show(dir, "HEAD", "p.go").
		Return([]byte(committed), nil)
	code, at, errs := with.call("measure", "--at", "HEAD", "mine="+dir)
	if code != exitOK {
		t.Fatalf("measure --at exited %d: %s", code, errs)
	}
	if !regexp.MustCompile(`sentences\s+4\n`).MatchString(disk) {
		t.Fatalf("the disk should hold four sentences:\n%s", disk)
	}
	if !regexp.MustCompile(`sentences\s+2\n`).MatchString(at) {
		t.Fatalf("HEAD should hold two sentences:\n%s", at)
	}
}

func TestMeasureRefusesWithNothingToMeasure(t *testing.T) {
	code, out, errs := newRig(t).call("measure")
	if code != exitFailed || out != "" ||
		errs != shape("measure", "okapi measure NAME=PATH ...") {
		t.Fatalf("measure with nothing gave %d, %q, %q", code, out, errs)
	}
}

func TestMeasureRefusesWhatItCannotRead(t *testing.T) {
	missing := "mine=" + filepath.Join(t.TempDir(), "nope")
	code, out, errs := newRig(t).call("measure", missing)
	if !failed(code, out, errs, "measure") {
		t.Fatalf("a missing tree gave %d, %q", code, errs)
	}
	with := newRig(t)
	with.repo.EXPECT().List("tree", "no-such-revision").
		Return(nil, errors.New("unknown revision"))
	code, out, errs = with.call("measure", "--at", "no-such-revision",
		"mine=tree")
	if !failed(code, out, errs, "measure") {
		t.Fatalf("a missing revision gave %d, %q", code, errs)
	}
}

func TestBlocksIsReachedThroughPzmWithItsFiles(t *testing.T) {
	code, out, errs := newRig(t).call("blocks", "testdata/tree/plain.go")
	if code != exitOK || errs != "" {
		t.Fatalf("blocks exited %d: %q", code, errs)
	}
	want := "== testdata/tree/plain.go:6-7  long, commas, chain, opener\n" +
		"// A habit opens this sentence, and it chains on, and on again, until\n" +
		"// nobody is left.\n"
	if out != want {
		t.Fatalf("blocks said:\n%s\nwant:\n%s", out, want)
	}
}

func TestBlocksHidesABlockThatIsOnlyLongUntilAll(t *testing.T) {
	_, plain, _ := newRig(t).call("blocks", "testdata/tree/plain.go")
	_, all, _ := newRig(t).call("blocks", "--all", "testdata/tree/plain.go")
	if strings.Contains(plain, "Long says") {
		t.Fatalf("blocks showed a block that is only long:\n%s", plain)
	}
	if !strings.Contains(all, "plain.go:10-11  long\n// Long says") {
		t.Fatalf("blocks --all hid the long block:\n%s", all)
	}
}

func TestBlocksHistoryNamesMoreWordsAndLeavesOthersAlone(t *testing.T) {
	_, plain, _ := newRig(t).call("blocks", "testdata/tree/plain.go")
	if strings.Contains(plain, "Zebra") {
		t.Fatalf("blocks flagged zebra with no word for it:\n%s", plain)
	}
	_, out, _ := newRig(t).call("blocks", "--history", "Zebra, ,",
		"testdata/tree/plain.go")
	if !strings.Contains(out, "plain.go:14-14  history\n// Zebra stripes") {
		t.Fatalf("blocks --history Zebra missed it:\n%s", out)
	}
	_, out, _ = newRig(t).call("blocks", "--history=Giraffe",
		"testdata/tree/plain.go")
	if strings.Contains(out, "Zebra") {
		t.Fatalf("blocks --history Giraffe flagged zebra:\n%s", out)
	}
}

func TestBlocksMarksAHistoryComment(t *testing.T) {
	file := filepath.Join(t.TempDir(), "h.go")
	text := "package h\n\n// Fixed on 2026-01-02 after it was slow.\nfunc H() {}\n"
	if err := os.WriteFile(file, []byte(text), fileMode); err != nil {
		t.Fatal(err)
	}
	_, out, _ := newRig(t).call("blocks", file)
	if !strings.Contains(out, "history") {
		t.Fatalf("a dated comment was not marked history:\n%s", out)
	}
}

func TestBlocksRefusesWithNoFiles(t *testing.T) {
	code, out, errs := newRig(t).call("blocks")
	if code != exitFailed || out != "" ||
		errs != shape("blocks", "okapi blocks FILE ...") {
		t.Fatalf("blocks with nothing gave %d, %q, %q", code, out, errs)
	}
}

func TestBlocksRefusesAFileItCannotRead(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.go")
	code, out, errs := newRig(t).call("blocks", missing)
	if !failed(code, out, errs, "blocks") {
		t.Fatalf("a missing file gave %d, %q", code, errs)
	}
}

// drafting is a rig whose model is up and answers every chat with
// reply. The model source is asked for model, which may be blank. The
// chat must carry messages that asking accepts.
func drafting(t *testing.T, model, reply string, asking gomock.Matcher) rig {
	t.Helper()
	with := newRig(t)
	with.models.EXPECT().Client(model).Return(with.client)
	with.client.EXPECT().Up().Return(true)
	with.client.EXPECT().Model().Return("m")
	with.client.EXPECT().Chat(asking).Return(reply, nil)
	return with
}

func TestDraftIsReachedThroughPzmAndWritesTheDrafts(t *testing.T) {
	dir := habitTree(t)
	with := drafting(t, "", "// Habit opens plainly.", endsWith{lastOf(t)})
	target := filepath.Join(dir, "drafts.json")
	code, out, errs := with.call("draft", target, "x.go")
	if code != exitOK || errs != "" {
		t.Fatalf("draft exited %d: %q", code, errs)
	}
	want := "1 blocks to draft with m\n1 blocks drafted, written to " +
		target + "\n"
	if out != want {
		t.Fatalf("draft said %q, want %q", out, want)
	}
	blocks, err := draft.Read(target)
	if err != nil || len(blocks) != count.One ||
		blocks[count.First].After() != "// Habit opens plainly." {
		t.Fatalf("the drafts file held %v, %v", blocks, err)
	}
}

func TestDraftModelFlagIsHandedToTheModelSource(t *testing.T) {
	dir := habitTree(t)
	with := drafting(t, "from-flag", "// Plain.", gomock.Any())
	target := filepath.Join(dir, "d.json")
	code, _, errs := with.call("draft", "--model", "from-flag", target, "x.go")
	if code != exitOK {
		t.Fatalf("draft --model exited %d: %s", code, errs)
	}
}

func TestOllamaModelsAskTheEnvironmentUnlessNamed(t *testing.T) {
	t.Setenv("OKAPI_MODEL", "from-env")
	t.Setenv("OKAPI_OLLAMA", "http://ollama.test/api/chat")
	if got := (ollamaModels{}).Client("").Model(); got != "from-env" {
		t.Errorf("no model named asked %q, want the environment's", got)
	}
	if got := (ollamaModels{}).Client("big").Model(); got != "big" {
		t.Errorf("a named model asked %q, want big", got)
	}
}

func TestDraftVoiceReplacesTheBuiltInVoice(t *testing.T) {
	dir := habitTree(t)
	voices := filepath.Join(dir, "voice")
	if err := os.Mkdir(voices, dirMode); err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join(voices, "rules.txt")
	shots := filepath.Join(voices, "shots.txt")
	if err := os.WriteFile(rules, []byte("THE RULES"), fileMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shots,
		[]byte("== before\n// one\n== after\n// two\n"), fileMode); err != nil {
		t.Fatal(err)
	}
	asking := leadsWith{ollama.System("THE RULES")}
	with := drafting(t, "", "// Plain.", asking)
	target := filepath.Join(dir, "d.json")
	code, _, errs := with.call("draft", "--voice", voices, target, "x.go")
	if code != exitOK {
		t.Fatalf("draft --voice exited %d: %s", code, errs)
	}
}

func TestDraftWithoutVoiceUsesTheBuiltInVoice(t *testing.T) {
	dir := habitTree(t)
	rules := leadsWith{ollama.System("THE RULES")}
	asking := gomock.All(gomock.Not(rules), showsExamples{})
	with := drafting(t, "", "// Plain.", asking)
	code, _, errs := with.call("draft", filepath.Join(dir, "d.json"), "x.go")
	if code != exitOK {
		t.Fatalf("draft exited %d: %s", code, errs)
	}
}

func TestDraftDryRunSaysWhatItWouldDoAndDoesNone(t *testing.T) {
	dir := habitTree(t)
	with := newRig(t)
	with.models.EXPECT().Client("m").Return(with.client)
	with.client.EXPECT().Model().Return("m")
	target := filepath.Join(dir, "drafts.json")
	code, out, errs := with.call("draft", "--dry-run", "--model", "m", target, "x.go")
	if code != exitOK || errs != "" {
		t.Fatalf("draft --dry-run exited %d: %q", code, errs)
	}
	if want := "would draft 1 blocks with m to " + target + "\n"; out != want {
		t.Fatalf("draft --dry-run said %q, want %q", out, want)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("a dry run wrote the drafts file")
	}
}

func TestDraftRefusesWithNoFiles(t *testing.T) {
	code, out, errs := newRig(t).call("draft", "drafts.json")
	if code != exitFailed || out != "" ||
		errs != shape("draft", "okapi draft OUT.json FILE ...") {
		t.Fatalf("draft with no files gave %d, %q, %q", code, out, errs)
	}
}

func TestDraftRefusesAVoiceItCannotRead(t *testing.T) {
	dir := habitTree(t)
	nope := filepath.Join(dir, "nope")
	code, out, errs := newRig(t).call("draft", "--voice", nope,
		filepath.Join(dir, "d.json"), "x.go")
	if !failed(code, out, errs, "draft") {
		t.Fatalf("a missing voice gave %d, %q", code, errs)
	}
}

func TestDraftRefusesAFileItCannotRead(t *testing.T) {
	dir := habitTree(t)
	with := newRig(t)
	with.models.EXPECT().Client("").Return(with.client)
	with.client.EXPECT().Up().Return(true)
	code, out, errs := with.call("draft", filepath.Join(dir, "d.json"),
		"missing.go")
	if !failed(code, out, errs, "draft") {
		t.Fatalf("a missing file gave %d, %q", code, errs)
	}
}

func TestDraftRefusesATargetItCannotWrite(t *testing.T) {
	dir := habitTree(t)
	with := drafting(t, "", "// Plain.", gomock.Any())
	target := filepath.Join(dir, "no", "such", "d.json")
	code, out, errs := with.call("draft", target, "x.go")
	if code != exitFailed || !strings.HasPrefix(errs, "okapi draft: ") {
		t.Fatalf("an unwritable target gave %d, %q, %q", code, out, errs)
	}
}

func TestDraftSaysWhenTheModelIsNotThere(t *testing.T) {
	dir := habitTree(t)
	with := newRig(t)
	with.models.EXPECT().Client("").Return(with.client)
	with.client.EXPECT().Up().Return(false)
	with.client.EXPECT().URL().Return("http://ollama.test/api/chat")
	target := filepath.Join(dir, "drafts.json")
	code, out, errs := with.call("draft", target, "x.go")
	want := "okapi draft: ollama is not answering at http://ollama.test/api/chat\n"
	if code != exitFailed || out != "" || errs != want {
		t.Fatalf("draft gave %d, %q, %q; want %d, %q", code, out, errs,
			exitFailed, want)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("draft wrote a file with no model")
	}
}

func TestDraftKeepsWhatItDraftedWhenTheModelFails(t *testing.T) {
	dir := habitTree(t)
	with := newRig(t)
	with.models.EXPECT().Client("").Return(with.client)
	with.client.EXPECT().Up().Return(true)
	with.client.EXPECT().Model().Return("m")
	with.client.EXPECT().Chat(gomock.Any()).Return("", errors.New("tired"))
	target := filepath.Join(dir, "drafts.json")
	code, _, errs := with.call("draft", target, "x.go")
	if code != exitFailed ||
		!strings.Contains(errs, "0 of 1 drafted and written to "+target) {
		t.Fatalf("draft gave %d, %q", code, errs)
	}
	if blocks, err := draft.Read(target); err != nil ||
		len(blocks) != count.Nothing {
		t.Fatalf("the drafts file held %v, %v; want it written and empty",
			blocks, err)
	}
}

// draftsFile writes a drafts file for x.go with the draft on its one
// block, and returns its path.
func draftsFile(t *testing.T, after string) string {
	t.Helper()
	blocks := comment.Blocks("x.go", habit)
	blocks[count.First].Drafted(after)
	path := filepath.Join(t.TempDir(), "drafts.json")
	if err := draft.Write(path, blocks); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBriefIsReachedThroughPzmWithItsDrafts(t *testing.T) {
	drafts := draftsFile(t, "// Plain.")
	code, out, errs := newRig(t).call("brief", drafts)
	if code != exitOK || errs != "" {
		t.Fatalf("brief exited %d: %q", code, errs)
	}
	for _, want := range []string{"Review the comment drafts in " + drafts,
		"\n  x.go:3\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("brief lacks %q:\n%s", want, out)
		}
	}
}

func TestBriefRefusesWithNoDrafts(t *testing.T) {
	code, out, errs := newRig(t).call("brief")
	if code != exitFailed || out != "" ||
		errs != shape("brief", "okapi brief DRAFTS.json") {
		t.Fatalf("brief with nothing gave %d, %q, %q", code, out, errs)
	}
}

func TestBriefRefusesADraftsFileItCannotRead(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")
	code, out, errs := newRig(t).call("brief", missing)
	if !failed(code, out, errs, "brief") {
		t.Fatalf("a missing drafts file gave %d, %q, %q", code, out, errs)
	}
}

// reviewing writes a review text and returns its path.
func reviewing(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "review.txt")
	if err := os.WriteFile(path, []byte(text), fileMode); err != nil {
		t.Fatal(err)
	}
	return path
}

// applying is a rig whose tree holds habit as x.go under root, and
// expects the file to be written once.
func applying(t *testing.T, root string) (rig, *written) {
	t.Helper()
	with := newRig(t)
	path := filepath.Join(root, "x.go")
	with.files.EXPECT().Read(path).Return([]byte(habit), nil)
	put := &written{}
	with.files.EXPECT().Write(path, put).Return(nil)
	return with, put
}

func TestApplyIsReachedThroughPzmAndRewritesTheFile(t *testing.T) {
	drafts := draftsFile(t, "// Habit opens plainly.")
	review := reviewing(t, "x.go:3 accept\n")
	with, put := applying(t, "tree")
	code, out, errs := with.call("apply", "--repo", "tree", drafts, review)
	if code != exitOK || errs != "" {
		t.Fatalf("apply exited %d: %q", code, errs)
	}
	if out != "fix 0  accept 1  keep 0  missing 0\n" {
		t.Fatalf("apply said %q", out)
	}
	want := "package x\n\n// Habit opens plainly.\nfunc Habit() {}\n"
	if put.text != want {
		t.Fatalf("x.go became:\n%s\nwant:\n%s", put.text, want)
	}
}

func TestApplyDefaultsToTheCurrentTree(t *testing.T) {
	drafts := draftsFile(t, "// Habit opens plainly.")
	review := reviewing(t, "x.go:3 accept\n")
	with, _ := applying(t, ".")
	code, _, errs := with.call("apply", drafts, review)
	if code != exitOK {
		t.Fatalf("apply exited %d: %s", code, errs)
	}
}

func TestApplyTakesAVerdictByHandAfterTheReview(t *testing.T) {
	drafts := draftsFile(t, "// Habit opens plainly.")
	review := reviewing(t, "x.go:3 accept\n")
	with, put := applying(t, ".")
	code, out, errs := with.call("apply", drafts, review, "x.go:3=keep")
	if code != exitOK || errs != "" ||
		out != "fix 0  accept 0  keep 1  missing 0\n" {
		t.Fatalf("apply gave %d, %q, %q", code, out, errs)
	}
	if put.text != habit {
		t.Fatalf("a kept block changed the file:\n%s", put.text)
	}
}

func TestApplyDryRunSaysWhatItWouldDoAndDoesNone(t *testing.T) {
	drafts := draftsFile(t, "// Habit opens plainly.")
	review := reviewing(t, "x.go:3 accept\n")
	code, out, errs := newRig(t).call("apply", "--dry-run", drafts, review)
	if code != exitOK || errs != "" {
		t.Fatalf("apply --dry-run exited %d: %q", code, errs)
	}
	if out != "would apply the review to 1 blocks under .\n" {
		t.Fatalf("apply --dry-run said %q", out)
	}
}

func TestApplyRefusesWithTooFewFiles(t *testing.T) {
	drafts := draftsFile(t, "// Plain.")
	code, out, errs := newRig(t).call("apply", drafts)
	if code != exitFailed || out != "" ||
		errs != shape("apply", "okapi apply DRAFTS.json REVIEW") {
		t.Fatalf("apply with one file gave %d, %q, %q", code, out, errs)
	}
}

func TestApplyRefusesWhatItCannotRead(t *testing.T) {
	drafts := draftsFile(t, "// Plain.")
	review := reviewing(t, "x.go:3 accept\n")
	nope := filepath.Join(t.TempDir(), "nope")
	cases := map[string][]string{
		"missing drafts": {"apply", nope, review},
		"missing review": {"apply", drafts, nope},
		"bad override":   {"apply", drafts, review, "x.go:3=burn"},
	}
	for name, args := range cases {
		code, out, errs := newRig(t).call(args...)
		if !failed(code, out, errs, "apply") {
			t.Errorf("%s gave %d, %q, %q", name, code, out, errs)
		}
	}
}

func TestApplyRefusesAMovedCommentAndWritesNothing(t *testing.T) {
	drafts := draftsFile(t, "// Plain.")
	review := reviewing(t, "x.go:3 accept\n")
	with := newRig(t)
	moved := "package x\n\nfunc Habit() {}\n"
	with.files.EXPECT().Read("x.go").Return([]byte(moved), nil)
	code, out, errs := with.call("apply", drafts, review)
	if !failed(code, out, errs, "apply") ||
		!strings.Contains(errs, "the comment is not what was drafted") {
		t.Fatalf("a moved comment gave %d, %q", code, errs)
	}
}

func TestVersionIsReachedThroughPzm(t *testing.T) {
	with := newRig(t)
	with.builds.EXPECT().Info().Return(&debug.BuildInfo{
		Main: debug.Module{Version: "v9.9.9"}}, true)
	code, out, errs := with.call("version")
	if code != exitOK || errs != "" || out != "okapi v9.9.9\n" {
		t.Fatalf("version gave %d, %q, %q", code, out, errs)
	}
}

func TestVersionSaysWhenThereIsNoBuildInformation(t *testing.T) {
	with := newRig(t)
	with.builds.EXPECT().Info().Return(nil, false)
	code, out, _ := with.call("version")
	if code != exitOK || out != "okapi, built without build information\n" {
		t.Fatalf("version gave %d, %q", code, out)
	}
}

func TestVersionTakesNoArguments(t *testing.T) {
	code, out, errs := newRig(t).call("version", "--at", "x")
	if code != exitUsage || out != "" || errs != "okapi: unknown flag: --at\n" {
		t.Fatalf("version --at gave %d, %q, %q", code, out, errs)
	}
}
