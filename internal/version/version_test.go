package version

import (
	"strings"
	"testing"
)

func setVersionVars(t *testing.T, version, commit, buildTime, goVersion string) {
	t.Helper()
	origVersion, origCommit, origBuildTime, origGoVersion := Version, CommitSHA, BuildTime, GoVersion
	t.Cleanup(func() {
		Version, CommitSHA, BuildTime, GoVersion = origVersion, origCommit, origBuildTime, origGoVersion
	})
	Version, CommitSHA, BuildTime, GoVersion = version, commit, buildTime, goVersion
}

func TestInfo(t *testing.T) {
	setVersionVars(t, "v1.2.3", "abc1234", "2026-01-02T03:04:05Z", "go1.26.0")

	got := Info()
	expected := "Version: v1.2.3\nCommit: abc1234\nBuild Time: 2026-01-02T03:04:05Z\nGo Version: go1.26.0"
	if got != expected {
		t.Errorf("Info() = %q, want %q", got, expected)
	}
}

func TestInfoDefaults(t *testing.T) {
	setVersionVars(t, "dev", "unknown", "unknown", "go1.26.0")

	got := Info()
	for _, want := range []string{"Version: dev", "Commit: unknown", "Build Time: unknown", "Go Version: go1.26.0"} {
		if !strings.Contains(got, want) {
			t.Errorf("Info() = %q, missing %q", got, want)
		}
	}
}

func TestShort(t *testing.T) {
	setVersionVars(t, "v9.8.7", "abc1234", "unknown", "go1.26.0")

	if got := Short(); got != "v9.8.7" {
		t.Errorf("Short() = %q, want %q", got, "v9.8.7")
	}
}
