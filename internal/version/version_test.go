// SPDX-License-Identifier: MIT

package version

import (
	"encoding/json"
	"runtime"
	"runtime/debug"
	"testing"
)

const rev = "0123456789abcdef0123456789abcdef01234567"

func TestFromBuildInfo(t *testing.T) {
	platform := runtime.GOOS + "/" + runtime.GOARCH
	vcs := func(modified string) []debug.BuildSetting {
		return []debug.BuildSetting{
			{Key: "-compiler", Value: "gc"},
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: rev},
			{Key: "vcs.time", Value: "2026-10-25T00:57:00Z"},
			{Key: "vcs.modified", Value: modified},
		}
	}
	tests := []struct {
		name    string
		ldflags string
		bi      *debug.BuildInfo
		ok      bool
		want    Info
	}{
		{
			name:    "ldflags version wins over the module version",
			ldflags: "v0.1.0",
			bi:      &debug.BuildInfo{GoVersion: "go1.27.3", Main: debug.Module{Version: "v0.0.9"}, Settings: vcs("false")},
			ok:      true,
			want:    Info{Version: "v0.1.0", Commit: rev, CommitTime: "2026-10-25T00:57:00Z", Go: "go1.27.3", Platform: platform},
		},
		{
			name:    "empty ldflags version falls back to the module version",
			ldflags: "",
			bi:      &debug.BuildInfo{GoVersion: "go1.27.3", Main: debug.Module{Version: "v0.2.0"}, Settings: vcs("false")},
			ok:      true,
			want:    Info{Version: "v0.2.0", Commit: rev, CommitTime: "2026-10-25T00:57:00Z", Go: "go1.27.3", Platform: platform},
		},
		{
			name:    "module version",
			ldflags: "dev",
			bi:      &debug.BuildInfo{GoVersion: "go1.27.3", Main: debug.Module{Version: "v0.2.0"}, Settings: vcs("false")},
			ok:      true,
			want:    Info{Version: "v0.2.0", Commit: rev, CommitTime: "2026-10-25T00:57:00Z", Go: "go1.27.3", Platform: platform},
		},
		{
			name:    "devel module version",
			ldflags: "dev",
			bi:      &debug.BuildInfo{GoVersion: "go1.27.3", Main: debug.Module{Version: "(devel)"}, Settings: vcs("false")},
			ok:      true,
			want:    Info{Version: "dev", Commit: rev, CommitTime: "2026-10-25T00:57:00Z", Go: "go1.27.3", Platform: platform},
		},
		{
			name:    "empty module version",
			ldflags: "dev",
			bi:      &debug.BuildInfo{GoVersion: "go1.27.3", Settings: vcs("false")},
			ok:      true,
			want:    Info{Version: "dev", Commit: rev, CommitTime: "2026-10-25T00:57:00Z", Go: "go1.27.3", Platform: platform},
		},
		{
			name:    "modified source tree",
			ldflags: "dev",
			bi:      &debug.BuildInfo{GoVersion: "go1.27.3", Main: debug.Module{Version: "v0.2.0"}, Settings: vcs("true")},
			ok:      true,
			want:    Info{Version: "v0.2.0", Commit: rev, CommitTime: "2026-10-25T00:57:00Z", Modified: true, Go: "go1.27.3", Platform: platform},
		},
		{
			name:    "missing settings",
			ldflags: "dev",
			bi:      &debug.BuildInfo{GoVersion: "go1.27.3", Main: debug.Module{Version: "v0.2.0"}},
			ok:      true,
			want:    Info{Version: "v0.2.0", Commit: "unknown", Go: "go1.27.3", Platform: platform},
		},
		{
			name:    "empty revision",
			ldflags: "dev",
			bi:      &debug.BuildInfo{GoVersion: "go1.27.3", Settings: []debug.BuildSetting{{Key: "vcs.revision"}}},
			ok:      true,
			want:    Info{Version: "dev", Commit: "unknown", Go: "go1.27.3", Platform: platform},
		},
		{
			name:    "missing Go version",
			ldflags: "dev",
			bi:      &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0"}},
			ok:      true,
			want:    Info{Version: "v0.2.0", Commit: "unknown", Go: runtime.Version(), Platform: platform},
		},
		{
			name:    "no build information",
			ldflags: "dev",
			want:    Info{Version: "dev", Commit: "unknown", Go: runtime.Version(), Platform: platform},
		},
		{
			name:    "no build information, ldflags version",
			ldflags: "v0.1.0",
			want:    Info{Version: "v0.1.0", Commit: "unknown", Go: runtime.Version(), Platform: platform},
		},
		{
			name:    "not ok is not trusted even with build information",
			ldflags: "dev",
			bi:      &debug.BuildInfo{GoVersion: "go1.27.3", Main: debug.Module{Version: "v0.2.0"}, Settings: vcs("true")},
			want:    Info{Version: "dev", Commit: "unknown", Go: runtime.Version(), Platform: platform},
		},
		{
			name:    "ok without build information",
			ldflags: "dev",
			ok:      true,
			want:    Info{Version: "dev", Commit: "unknown", Go: runtime.Version(), Platform: platform},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromBuildInfo(tt.ldflags, tt.bi, tt.ok); got != tt.want {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestInfoJSON(t *testing.T) {
	info := Info{Version: "v0.1.0", Commit: rev, Go: "go1.27.3", Platform: "linux/amd64"}
	b, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":"v0.1.0","commit":"` + rev + `","modified":false,"go":"go1.27.3","platform":"linux/amd64"}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
	info.CommitTime = "2026-10-25T00:57:00Z"
	b, err = json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	want = `{"version":"v0.1.0","commit":"` + rev + `","commit_time":"2026-10-25T00:57:00Z","modified":false,"go":"go1.27.3","platform":"linux/amd64"}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
}

func TestGet(t *testing.T) {
	info := Get()
	if info.Go == "" {
		t.Error("Go is empty")
	}
	if want := runtime.GOOS + "/" + runtime.GOARCH; info.Platform != want {
		t.Errorf("Platform = %q, want %q", info.Platform, want)
	}
	if info.Version == "" || info.Commit == "" {
		t.Errorf("Version or Commit is empty: %+v", info)
	}
	if again := Get(); again != info {
		t.Errorf("second call = %+v, want %+v", again, info)
	}
}
