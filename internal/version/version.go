// SPDX-License-Identifier: MIT

// Package version reports how an irekey binary was built: its release
// version, the commit it was built from, the Go toolchain and the platform.
package version

import (
	"runtime"
	"runtime/debug"
	"sync"
)

// Version is set at build time with -ldflags "-X github.com/hitoshyamamoto/irekey/internal/version.Version=v0.1.0".
// Builds without it (e.g. go install) fall back to the module version.
var Version = "dev"

// Info describes the running binary.
type Info struct {
	// Version is the release version, or "dev" when none is known.
	Version string `json:"version"`
	// Commit is the full revision the binary was built from, or "unknown".
	Commit string `json:"commit"`
	// CommitTime is the time of Commit in RFC 3339 format, when known.
	CommitTime string `json:"commit_time,omitempty"`
	// Modified reports whether the source tree had local modifications.
	Modified bool `json:"modified"`
	// Go is the version of the Go toolchain, e.g. "go1.27.3".
	Go string `json:"go"`
	// Platform is GOOS/GOARCH, e.g. "linux/amd64".
	Platform string `json:"platform"`
}

// FromBuildInfo builds an Info from the version set at build time and
// the result of debug.ReadBuildInfo. A version set at build time wins
// unless it is "dev" or empty; otherwise the module version is used,
// unless it is empty or "(devel)", in which case the version is "dev".
func FromBuildInfo(ldflagsVersion string, bi *debug.BuildInfo, ok bool) Info {
	info := Info{
		Version:  "dev",
		Commit:   "unknown",
		Go:       runtime.Version(),
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
	ok = ok && bi != nil
	switch {
	case ldflagsVersion != "dev" && ldflagsVersion != "":
		info.Version = ldflagsVersion
	case ok && bi.Main.Version != "" && bi.Main.Version != "(devel)":
		info.Version = bi.Main.Version
	}
	if !ok {
		return info
	}
	if bi.GoVersion != "" {
		info.Go = bi.GoVersion
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if s.Value != "" {
				info.Commit = s.Value
			}
		case "vcs.time":
			info.CommitTime = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	return info
}

var get = sync.OnceValue(func() Info {
	bi, ok := debug.ReadBuildInfo()
	return FromBuildInfo(Version, bi, ok)
})

// Get returns the Info of the running binary. It is computed on the
// first call and cached.
func Get() Info {
	return get()
}
