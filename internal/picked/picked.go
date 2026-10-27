// Package picked holds the numbers okapi uses that someone chose. They
// live in picked-numbers.yml, each with what it is and who picked it.
// The code asks for one by name.
package picked

import (
	_ "embed"
	"fmt"
	"math"
	"time"

	"github.com/janearc/puzzlemouth/numbers"
)

// file is the picked numbers' file, as a refusal names it.
const file = "picked-numbers.yml"

// raw is the picked numbers as the file holds them.
//
//go:embed picked-numbers.yml
var raw []byte

// Value is a picked number by its name. If the file does not hold the
// name the program stops and says which. That is a mistake in okapi's
// own code, which its tests find.
func Value(name string) float64 {
	// If the file does not read there is nothing to find, and the panic
	// below says so. The tests ask for every name.
	chosen, _ := numbers.Read(file, raw)
	number, err := chosen.Get(name)
	if err != nil {
		panic(fmt.Errorf("okapi: %w", err))
	}
	return number.Value()
}

// Whole is a picked number that must be a whole count. One that is not
// stops the program, as a missing name does.
func Whole(name string) int {
	value := Value(name)
	if value != math.Trunc(value) {
		panic(fmt.Errorf("okapi: %s is %v, not whole", name, value))
	}
	return int(value)
}

// Span is a picked number of a unit of time, such as minutes.
func Span(name string, unit time.Duration) time.Duration {
	return time.Duration(Whole(name)) * unit
}
