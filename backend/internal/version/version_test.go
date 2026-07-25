package version

import (
	"runtime/debug"
	"testing"
)

func TestStringDescribesTheBuild(t *testing.T) {
	settings := func(kv ...string) func() (*debug.BuildInfo, bool) {
		info := &debug.BuildInfo{}
		for i := 0; i+1 < len(kv); i += 2 {
			info.Settings = append(info.Settings, debug.BuildSetting{Key: kv[i], Value: kv[i+1]})
		}
		return func() (*debug.BuildInfo, bool) { return info, true }
	}
	cases := []struct {
		name string
		read func() (*debug.BuildInfo, bool)
		want string
	}{
		{"no build info", func() (*debug.BuildInfo, bool) { return nil, false }, "1.0.0"},
		{"no vcs stamp", settings("GOOS", "linux"), "1.0.0"},
		{"clean tree", settings("vcs.revision", "84020fd6aff1104ae3", "vcs.modified", "false"), "1.0.0 (84020fd)"},
		{"dirty tree", settings("vcs.revision", "84020fd6aff1104ae3", "vcs.modified", "true"), "1.0.0 (84020fd, dirty)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := build("1.0.0", tc.read); got != tc.want {
				t.Fatalf("build() = %q, want %q", got, tc.want)
			}
		})
	}
}
