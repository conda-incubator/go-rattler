//go:build cgo

package gorattler

import (
	"fmt"

	"github.com/dbast/go-rattler/internal/api"
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
	conda, pypi, status := api.LockEnvironmentCounts(lockPath, name, platform)
	switch status {
	case 0:
		return PackageCounts{Conda: conda, PyPI: pypi}, nil
	case 2:
		return PackageCounts{}, fmt.Errorf("cannot read pixi lockfile %q", lockPath)
	case 3:
		return PackageCounts{}, fmt.Errorf("environment %q is absent from pixi lockfile", name)
	case 4:
		return PackageCounts{}, fmt.Errorf("platform %q is absent from locked environment %q", platform, name)
	default:
		return PackageCounts{}, fmt.Errorf("rattler lock lookup failed (status %d)", status)
	}
}
