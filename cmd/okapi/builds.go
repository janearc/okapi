package main

import "runtime/debug"

// buildSource is where okapi learns which build it is.
type buildSource interface {
	// Info is what the build knew of itself, and whether it knew.
	Info() (*debug.BuildInfo, bool)
}

// runtimeBuilds reads the information the Go toolchain put in this
// binary.
type runtimeBuilds struct{}

// Info is the build information of the running binary.
func (runtimeBuilds) Info() (*debug.BuildInfo, bool) {
	return debug.ReadBuildInfo()
}
