// Package count names the numbers a count is read against, so no
// package writes a bare 0 or 1 to ask whether there is anything or to
// step past a fencepost.
package count

// Nothing is the count of an empty thing, and where a count starts:
// defined so.
const Nothing = 0

// First is where a list begins, as go has defined it: nothing in.
const First = 0

// One is a single thing, and the step from one place to the next:
// defined so.
const One = 1

// Origin is where lines and columns are counted from, as go/token has
// defined them: one.
const Origin = 1

// Pair is two things, and the place of a list's third: defined so.
const Pair = 2
