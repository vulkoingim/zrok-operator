// Package build holds version metadata.
//
// Version and Date are stamped via -ldflags. GitRevision is filled from
// runtime/debug (Go 1.18+ vcs.revision / vcs.modified) when unset — that is
// the default for mise/GoReleaser (-buildvcs=true). Docker has no .git, so it
// still passes -X GitRevision=.
package build

import "runtime/debug"

var (
	Version     = "dev"
	GitRevision = ""
	Date        = "unknown"
)

func init() {
	if GitRevision == "" {
		GitRevision = vcsRevision()
	}
}

func vcsRevision() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "none"
	}
	return revisionFromSettings(bi.Settings)
}

func revisionFromSettings(settings []debug.BuildSetting) string {
	var rev, modified string
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if rev == "" {
		return "none"
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if modified == "true" {
		return rev + "-dirty"
	}
	return rev
}
