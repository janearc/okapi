package picked_test

import (
	"testing"
	"time"

	"github.com/janearc/okapi/internal/count"
	"github.com/janearc/okapi/internal/picked"
)

// noteRecover sets stopped when the program was stopping. It is the
// deferred function, so recover is called directly by it.
func noteRecover(stopped *bool) { *stopped = recover() != nil }

// stops is whether the work stops the program.
func stops(work func()) (stopped bool) {
	defer noteRecover(&stopped)
	work()
	return stopped
}

// doNothing does nothing.
func doNothing() {}

// askForUnknownName asks for a number the file does not hold.
func askForUnknownName() { picked.Value("no-such-number") }

// askForFractionAsCount asks for a fraction as a whole count.
func askForFractionAsCount() { picked.Whole("temperature") }

// every number the code asks for is in the file and is above nothing.
func TestTheNumbersOkapiAsksForAreHeld(t *testing.T) {
	for _, name := range []string{"answer-wait", "up-wait", "temperature",
		"progress-every", "code-lines", "shortest-sentence"} {
		if picked.Value(name) <= count.Nothing {
			t.Errorf("%s is not above nothing", name)
		}
	}
}

func TestWholeIsTheValueAsACount(t *testing.T) {
	whole := float64(picked.Whole("progress-every"))
	if whole != picked.Value("progress-every") {
		t.Error("a whole count differs from its value")
	}
}

func TestAnUnknownNameStopsTheProgram(t *testing.T) {
	if !stops(askForUnknownName) {
		t.Error("an unknown name did not stop the program")
	}
}

func TestAFractionIsNotACount(t *testing.T) {
	if !stops(askForFractionAsCount) {
		t.Error("a fraction was read as a count")
	}
}

func TestAHeldNameDoesNotStopTheProgram(t *testing.T) {
	if stops(doNothing) {
		t.Error("the program stopped for no reason")
	}
}

func TestSpanIsTheNumberOfThatUnit(t *testing.T) {
	want := time.Duration(picked.Whole("up-wait")) * time.Second
	if got := picked.Span("up-wait", time.Second); got != want {
		t.Errorf("span %v, want %v", got, want)
	}
}
