package version

import (
	"testing"
)

func TestInfo(t *testing.T) {
	info := Info()

	if info["version"] == "" {
		t.Error("expected non-empty version")
	}
	if info["go_version"] == "" {
		t.Error("expected non-empty go_version")
	}
	if _, ok := info["build_time"]; !ok {
		t.Error("expected build_time key")
	}
	if _, ok := info["git_commit"]; !ok {
		t.Error("expected git_commit key")
	}
}

func TestGoVersion(t *testing.T) {
	gv := GoVersion()
	if gv == "" {
		t.Error("expected non-empty Go version")
	}
}
