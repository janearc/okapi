package count_test

import (
	"testing"

	"github.com/janearc/okapi/internal/count"
)

func TestCountsAreWhatGoDefinesThem(t *testing.T) {
	if count.Nothing != len("") || count.First != len("") {
		t.Errorf("nothing is %d and first is %d, want the length of nothing",
			count.Nothing, count.First)
	}
	if count.One != len("x") || count.Origin != len("x") {
		t.Errorf("one is %d and the origin is %d, want the length of x",
			count.One, count.Origin)
	}
}

func TestCountsAreNotEachOther(t *testing.T) {
	if count.Nothing == count.One {
		t.Error("nothing is one")
	}
	if count.Pair != count.One+count.One {
		t.Error("a pair is not two ones")
	}
	if count.First == count.Origin {
		t.Error("a list and a line begin in the same place")
	}
}
