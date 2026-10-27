package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stampedCommit is the commit the end-to-end binary is stamped with.
const stampedCommit = "abc1234"

// binary builds okapi into the tree's bin directory and returns its
// path. The build is stamped as game stamps it.
func binary(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "bin", "okapi"))
	if err != nil {
		t.Fatal(err)
	}
	stamp := "-X main.build=" + stampedCommit +
		" -X main.built=2026-10-26T12:00:00Z"
	build := exec.Command("go", "build", "-ldflags", stamp, "-o", path, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v: %s", err, out)
	}
	return path
}

// run runs the binary with the words and returns its output and exit
// code.
func run(t *testing.T, path string, words ...string) (string, int) {
	t.Helper()
	out, err := exec.Command(path, words...).Output()
	if exited, ok := err.(*exec.ExitError); ok {
		return string(out), exited.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(out), exitOK
}

func TestBinaryTellsItsAge(t *testing.T) {
	out, code := run(t, binary(t), "--age")
	if code != exitOK || !strings.Contains(out, stampedCommit) {
		t.Errorf("okapi --age exited %d, said %q", code, out)
	}
}

func TestBinaryTellsAnAgentWhatItDoes(t *testing.T) {
	out, code := run(t, binary(t), "--agent-help")
	if code != exitOK {
		t.Fatalf("okapi --agent-help exited %d", code)
	}
	for _, want := range []string{"tool okapi: ", "verb measure: ",
		"verb apply: ", "exit 0 ran, 1 the verb failed, 2 nothing ran"} {
		if !strings.Contains(out, want) {
			t.Errorf("okapi --agent-help lacks %q:\n%s", want, out)
		}
	}
}

func TestBinaryHelpsOneVerb(t *testing.T) {
	out, code := run(t, binary(t), "measure", "--help")
	if code != exitOK || !strings.Contains(out, "okapi measure [ARGS]: ") ||
		!strings.Contains(out, "--at") {
		t.Errorf("okapi measure --help exited %d, said %q", code, out)
	}
}

func TestBinaryMeasuresATree(t *testing.T) {
	out, code := run(t, binary(t), "measure", "mine=testdata/tree")
	want := "                     mine\nfiles                   2\n" +
		"sentences               6\ncommas a sentence    0.67\n" +
		"two or more commas  16.7%\nopen with a or an   16.7%\n" +
		"chain clauses       16.7%\n"
	if code != exitOK || out != want {
		t.Errorf("okapi measure exited %d, said:\n%s", code, out)
	}
}

func TestBinaryRefusesAnUnknownVerbWithTwo(t *testing.T) {
	out, code := run(t, binary(t), "frobnicate")
	if code != exitUsage || out != "" {
		t.Errorf("okapi frobnicate exited %d, said %q", code, out)
	}
}
