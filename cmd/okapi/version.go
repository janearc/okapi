package main

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/janearc/puzzlemouth/pzm"
)

// versionPage is what okapi version says of itself.
const versionPage = `# okapi version

okapi version says which okapi this is.
`

// unknownBuild is what version says when the build knew nothing.
const unknownBuild = "okapi, built without build information"

// versionVerb is okapi version: which okapi this is.
func (tool okapi) versionVerb(call pzm.Call) error {
	info, ok := tool.builds.Info()
	if !ok {
		fmt.Fprintln(call.Out(), unknownBuild)
		return nil
	}
	described := describeBuild(info, time.Now())
	fmt.Fprintln(call.Out(), "okapi", described)
	return nil
}

// shortRev is how many characters of a commit name are shown.
const shortRev = 7 // defined by git: the shortest abbreviation of a commit

// describeBuild says which build this is. It gives the module version
// when there is one. Otherwise it gives the commit and its age, and
// says if the tree was dirty.
func describeBuild(info *debug.BuildInfo, now time.Time) string {
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	rev, when, dirty := "", "", ""
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			rev = setting.Value
		case "vcs.time":
			when = setting.Value
		case "vcs.modified":
			if setting.Value == "true" {
				dirty = ", with changes"
			}
		}
	}
	if rev == "" {
		return "devel"
	}
	if len(rev) > shortRev {
		rev = rev[:shortRev]
	}
	if at, err := time.Parse(time.RFC3339, when); err == nil {
		rev += fmt.Sprintf(
			", committed %s ago", now.Sub(at).Round(time.Minute),
		)
	}
	return rev + dirty
}
