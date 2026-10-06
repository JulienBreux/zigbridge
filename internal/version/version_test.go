package version_test

import (
	"strings"
	"testing"

	"github.com/julienbreux/zigbridge/internal/version"
)

func TestVersionInfo(t *testing.T) {
	origVer, origCommit := version.Version, version.Commit
	t.Cleanup(func() {
		version.Version = origVer
		version.Commit = origCommit
	})

	version.Version = "v1.2.3"
	version.Commit = "abcdef"
	info := version.Info()
	if !strings.Contains(info, "v1.2.3") || !strings.Contains(info, "abcdef") {
		t.Errorf("expected version info to contain version and commit, got: %s", info)
	}

	version.Commit = "none"
	if info = version.Info(); info != "v1.2.3" {
		t.Errorf("expected version info without commit to be 'v1.2.3', got: %s", info)
	}
}
