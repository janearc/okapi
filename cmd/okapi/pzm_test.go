package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
)

// asMain is the variable that makes this test binary run main.
const asMain = "OKAPI_TEST_AS_MAIN"

// TestMain runs main when it is asked to, rather than the tests. That
// is how the tests see main's exit code. Since pzm reads
// ~/.config/okapi/config, HOME points at a place in the temporary
// directory that holds nothing, so no test reads a real config. The
// built binary and the child processes inherit it.
func TestMain(suite *testing.M) {
	empty := filepath.Join(os.TempDir(), "okapi-tests-have-no-home")
	os.Setenv("HOME", empty)
	if os.Getenv(asMain) != "" {
		main()
		return
	}
	os.Exit(suite.Run())
}

// mainExits is the code this binary exits with when run as okapi with
// the words.
func mainExits(t *testing.T, words ...string) (int, string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(self, words...)
	child.Env = append(os.Environ(), asMain+"=1")
	out, err := child.Output()
	if exited, ok := err.(*exec.ExitError); ok {
		return exited.ExitCode(), string(out)
	}
	if err != nil {
		t.Fatal(err)
	}
	return exitOK, string(out)
}

func TestMainRunsTheWordsAfterItsName(t *testing.T) {
	code, out := mainExits(t, "version")
	if code != exitOK || !strings.HasPrefix(out, "okapi ") {
		t.Errorf("okapi version exited %d, said %q", code, out)
	}
}

func TestMainExitsTwoForNothingToRun(t *testing.T) {
	if code, _ := mainExits(t); code != exitUsage {
		t.Errorf("okapi with nothing exited %d, want %d", code, exitUsage)
	}
	if code, _ := mainExits(t, "frobnicate"); code != exitUsage {
		t.Errorf("okapi frobnicate exited %d, want %d", code, exitUsage)
	}
}

func TestMainExitsOneWhenAVerbFails(t *testing.T) {
	if code, _ := mainExits(t, "measure"); code != exitFailed {
		t.Errorf("okapi measure exited %d, want %d", code, exitFailed)
	}
}

func TestToolDeclarationsAreSound(t *testing.T) {
	if err := newRig(t).tool.pzmTool().Check(); err != nil {
		t.Fatal(err)
	}
}

func TestEveryVerbHasAnExampleThatRuns(t *testing.T) {
	if err := newRig(t).tool.pzmTool().Examples(os.DirFS(".")); err != nil {
		t.Fatal(err)
	}
}

func TestToolDeclaresTheSixVerbsAndPzmSpeaksForThem(t *testing.T) {
	code, out, errs := newRig(t).call("--agent-help")
	if code != exitOK || errs != "" {
		t.Fatalf("--agent-help exited %d: %q", code, errs)
	}
	for _, name := range []string{"measure", "blocks", "draft", "brief",
		"apply", "version"} {
		if !strings.Contains(out, "verb "+name+": ") {
			t.Errorf("--agent-help has no verb %s:\n%s", name, out)
		}
		if !strings.Contains(out, "facts "+name+": ") {
			t.Errorf("--agent-help states no facts for %s", name)
		}
	}
}

func TestOnlyWritingVerbsAreGivenDryRun(t *testing.T) {
	_, out, _ := newRig(t).call("--agent-help")
	for _, name := range []string{"draft", "apply"} {
		if !strings.Contains(out, "flag "+name+" --dry-run") {
			t.Errorf("%s has no --dry-run", name)
		}
	}
	for _, name := range []string{"measure", "blocks", "brief", "version"} {
		if strings.Contains(out, "flag "+name+" --dry-run") {
			t.Errorf("%s writes nothing yet has --dry-run", name)
		}
	}
}

func TestAgeSaysWhichBuildThisIs(t *testing.T) {
	code, out, _ := newRig(t).call("--age")
	if code != exitOK || !strings.Contains(out, "built by go rather than by game") {
		t.Fatalf("--age gave %d, %q; want an unstamped build to say so",
			code, out)
	}
}

func TestVerbHelpNamesTheVerbAndItsFlags(t *testing.T) {
	code, out, _ := newRig(t).call("blocks", "--help")
	if code != exitOK {
		t.Fatalf("blocks --help exited %d", code)
	}
	for _, want := range []string{"okapi blocks [ARGS]: ", "--all",
		"--history"} {
		if !strings.Contains(out, want) {
			t.Errorf("blocks --help lacks %q:\n%s", want, out)
		}
	}
}

func TestNoVerbPrintsTheHelpOnStderrAndExitsTwo(t *testing.T) {
	code, out, errs := newRig(t).call()
	if code != exitUsage || out != "" {
		t.Fatalf("no verb gave %d, stdout %q; want %d and nothing", code,
			out, exitUsage)
	}
	if !strings.Contains(errs, "okapi: makes the comments in a Go tree plain") ||
		!strings.Contains(errs, "measure") {
		t.Fatalf("no verb printed no help:\n%s", errs)
	}
}

func TestUnknownVerbFailsTheWayPzmFails(t *testing.T) {
	code, out, errs := newRig(t).call("frobnicate")
	if code != exitUsage || out != "" {
		t.Fatalf("unknown verb gave %d, stdout %q; want %d and nothing",
			code, out, exitUsage)
	}
	if errs != "okapi: there is no verb frobnicate\n" {
		t.Fatalf("unknown verb said %q", errs)
	}
}

func TestUnknownFlagIsRefusedByPzmForEachVerb(t *testing.T) {
	for _, name := range []string{"measure", "blocks", "draft", "brief",
		"apply", "version"} {
		code, out, errs := newRig(t).call(name, "--bogus")
		if code != exitUsage || out != "" ||
			errs != "okapi: unknown flag: --bogus\n" {
			t.Errorf("%s --bogus gave %d, %q, %q", name, code, out, errs)
		}
	}
}

func TestOldWaysOfAskingStillWork(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-help"}, {"-h"},
		{"--help"}} {
		code, out, _ := newRig(t).call(args...)
		if code != exitOK || !strings.Contains(out, "measure") {
			t.Errorf("okapi %v gave %d, %q", args, code, out)
		}
	}
}

func TestOldDashedVerbStillRuns(t *testing.T) {
	with := newRig(t)
	with.builds.EXPECT().Info().Return(&debug.BuildInfo{
		Main: debug.Module{Version: "v1.2.3"}}, true)
	code, out, _ := with.call("-version")
	if code != exitOK || out != "okapi v1.2.3\n" {
		t.Errorf("okapi -version gave %d, %q", code, out)
	}
}

func TestNormalizeKeepsTheOldSpellings(t *testing.T) {
	cases := []struct{ args, want []string }{
		{[]string{"help"}, []string{"--help"}},
		{[]string{"-help"}, []string{"--help"}},
		{[]string{"--measure", "x"}, []string{"measure", "x"}},
		{[]string{"-apply", "-repo", "d", "a", "b"},
			[]string{"apply", "--repo", "d", "a", "b"}},
		{[]string{"blocks", "-all", "-history=zebra", "f.go"},
			[]string{"blocks", "--all", "--history=zebra", "f.go"}},
		{[]string{"draft", "-model", "m", "o.json", "f.go"},
			[]string{"draft", "--model", "m", "o.json", "f.go"}},
		{[]string{"draft", "-voice=d", "o", "f"},
			[]string{"draft", "--voice=d", "o", "f"}},
		{[]string{"measure", "-at", "HEAD", "t"},
			[]string{"measure", "--at", "HEAD", "t"}},
	}
	tool := newRig(t).tool
	for _, each := range cases {
		if got := tool.normalize(each.args); !slices.Equal(got, each.want) {
			t.Errorf("%q became %q, want %q", each.args, got, each.want)
		}
	}
}

func TestNormalizeLeavesWhatItDoesNotKnowAlone(t *testing.T) {
	cases := [][]string{
		{},
		{"measure", "-x", "t"},
		{"blocks", "--", "-all"},
		{"measure", "mine=-at"},
		{"frobnicate"},
		{"blocks", "-a", "f.go"},
	}
	tool := newRig(t).tool
	for _, args := range cases {
		if got := tool.normalize(args); !slices.Equal(got, args) {
			t.Errorf("%q became %q, want it as it was", args, got)
		}
	}
}

func TestNormalizeDoesNotChangeTheCallersSlice(t *testing.T) {
	args := []string{"help"}
	newRig(t).tool.normalize(args)
	if !slices.Equal(args, []string{"help"}) {
		t.Fatalf("the caller's slice became %q", args)
	}
}

func TestDescribeBuildSaysWhichBuildThisIs(t *testing.T) {
	now := mustTime(t, "2026-10-26T12:00:00Z")
	cases := []struct {
		name string
		info debug.BuildInfo
		want string
	}{
		{"a module version", debug.BuildInfo{
			Main: debug.Module{Version: "v1.2.3"}}, "v1.2.3"},
		{"no version, no commit", debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"}}, "devel"},
		{"a commit", debug.BuildInfo{Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef"},
			{Key: "vcs.time", Value: "2026-10-26T10:30:00Z"},
		}}, "0123456, committed 1h30m0s ago"},
		{"a dirty short commit", debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc"},
				{Key: "vcs.modified", Value: "true"},
				{Key: "vcs.other", Value: "x"},
			}}, "abc, with changes"},
		{"a clean commit", debug.BuildInfo{Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc"},
			{Key: "vcs.modified", Value: "false"},
			{Key: "vcs.time", Value: "not a time"},
		}}, "abc"},
	}
	for _, each := range cases {
		if got := describeBuild(&each.info, now); got != each.want {
			t.Errorf("%s: described %q, want %q", each.name, got, each.want)
		}
	}
}

func TestRuntimeBuildsKnowTheRunningBinary(t *testing.T) {
	info, ok := runtimeBuilds{}.Info()
	if !ok || info.Path == "" {
		t.Errorf("the running binary knew nothing of itself: %v, %v", info, ok)
	}
}
