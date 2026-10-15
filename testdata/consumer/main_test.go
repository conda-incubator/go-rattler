package consumer

import (
	"path/filepath"
	"testing"

	gorattler "github.com/dbast/go-rattler"
)

func TestBothBindingsFromAnotherModule(t *testing.T) {
	comparison, err := gorattler.CompareVersions("1.0rc1", "1.0")
	if err != nil || comparison != -1 {
		t.Fatalf("CompareVersions() = %d, %v", comparison, err)
	}
	counts, err := gorattler.LockEnvironment(filepath.Join("..", "pixi.lock"), "default", "osx-arm64")
	if err != nil || counts != (gorattler.PackageCounts{Conda: 1, PyPI: 1}) {
		t.Fatalf("LockEnvironment() = %+v, %v", counts, err)
	}
}
