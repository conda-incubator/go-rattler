//go:build cgo

package gorattler

import (
	"fmt"
	"os"
)

// PackageCounts holds package counts from one pixi.lock environment/platform.
type PackageCounts struct {
	Conda int
	PyPI  int
}

// LockEnvironment counts locked packages; it does not inspect an installed prefix.
func LockEnvironment(lockPath, name, platform string) (PackageCounts, error) {
	if lockPath == "" || name == "" || platform == "" {
		return PackageCounts{}, fmt.Errorf("lock path, environment, and platform are required")
	}
	document, err := os.ReadFile(lockPath)
	if err != nil {
		return PackageCounts{}, fmt.Errorf("cannot read pixi lockfile %q: %w", lockPath, err)
	}
	packages, err := LockPackages(document, name, platform)
	if err != nil {
		return PackageCounts{}, fmt.Errorf("lock environment: %w", err)
	}
	var counts PackageCounts
	for _, p := range packages {
		switch p.Kind {
		case "conda":
			counts.Conda++
		case "pypi":
			counts.PyPI++
		}
	}
	return counts, nil
}
