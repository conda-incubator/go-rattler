package gorattler

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLockEnvironment(t *testing.T) {
	lock := filepath.Join("testdata", "pixi.lock")
	counts, err := LockEnvironment(lock, "default", "osx-arm64")
	if err != nil || counts != (PackageCounts{Conda: 1, PyPI: 1}) {
		t.Fatalf("LockEnvironment() = %+v, %v", counts, err)
	}
	for _, tt := range []struct {
		name, platform, want string
	}{
		{"missing", "osx-arm64", "environment"},
		{"default", "linux-64", "platform"},
	} {
		_, err := LockEnvironment(lock, tt.name, tt.platform)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("lookup %s/%s error = %v, want %q", tt.name, tt.platform, err, tt.want)
		}
	}
	if _, err := LockEnvironment("missing.lock", "default", "osx-arm64"); err == nil {
		t.Fatal("missing lockfile must return an error")
	}
}
