// Package version records which build of Agent Hub is running.
//
// A self-hosted product has to be able to answer "what am I running?" before an
// operator can match their instance against a changelog or a bug report.
package version

import (
	"runtime/debug"
	"strings"
	"sync"
)

// Version is the release this binary claims to be. It is bumped in the same
// commit that tags a release (see CHANGELOG.md), and can be overridden at build
// time for snapshot builds:
//
//	go build -ldflags "-X github.com/akadal/agent-hub/backend/internal/version.Version=1.1.0-rc1"
var Version = "1.1.0"

var (
	once   sync.Once
	cached string
)

// String is Version plus the VCS revision Go stamps into the binary, e.g.
// "1.0.0 (84020fd, dirty)". The revision is what actually distinguishes two
// builds carrying the same release number.
func String() string {
	once.Do(func() { cached = build(Version, debug.ReadBuildInfo) })
	return cached
}

func build(v string, read func() (*debug.BuildInfo, bool)) string {
	info, ok := read()
	if !ok {
		return v
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return v
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	parts := []string{rev}
	if dirty {
		parts = append(parts, "dirty")
	}
	return v + " (" + strings.Join(parts, ", ") + ")"
}
